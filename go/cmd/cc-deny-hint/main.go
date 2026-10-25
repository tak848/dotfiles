// cc-deny-hint は permissions.deny に当たりうる Bash コマンドに、その理由を additionalContext で添える
// PreToolUse hook。ルールは引数の JSON（dot_claude/permission-rules.libsonnet が生成）で受け取る。
//
// hooks の `if` で同じことをすると、コマンド名より先まで指定したパターンは `$()`・バッククォート・`$VAR` を
// 含むだけで常に hook が実行される（公式 docs の hooks "How if patterns match Bash commands"）。そのため
// 無関係なコマンドにも全ルールの理由が注入されていた。ここではコマンドを個々の単純コマンドに分けて照合し、
// 当たったルールの理由だけを返す。実際の拒否は permissions.deny が行うので、解析できないときは何も返さない。
package main

import (
	"encoding/json/v2"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
)

const (
	maxInput   = 1 << 20
	maxDepth   = 4
	hintPrefix = "[deny hint] このコマンドは permissions.deny にマッチする可能性があります。背景: "
)

// rule は permissions.deny の `Bash(<pattern>)` の pattern 部分と、その理由。
type rule struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
}

type input struct {
	ToolName  string `json:"tool_name"`
	ToolInput *struct {
		Command *string `json:"command"`
	} `json:"tool_input"`
}

// keywords は単純コマンドの先頭に来うる予約語。外してから照合する。
var keywords = map[string]bool{
	"!": true, "{": true, "if": true, "then": true, "elif": true, "else": true, "do": true, "while": true, "until": true,
}

// wrappers は後ろの単語をコマンドとして実行するラッパー。オプションと一緒に外してから照合する。
var wrappers = map[string]bool{
	"command": true, "builtin": true, "exec": true, "nohup": true, "sudo": true, "env": true, "nice": true, "time": true,
}

// wrapperValueOptions は値を取るラッパーのオプション。値はコマンドではないので、オプションと一緒に外す。
// env は BSD と GNU の man、sudo と nice は man、exec は bash の man で確認した。
var wrapperValueOptions = map[string][]string{
	"env":  {"-u", "--unset", "-C", "--chdir", "-P", "-S", "--split-string", "-a", "--argv0"},
	"sudo": {"-C", "--close-from", "-D", "--chdir", "-g", "--group", "-h", "--host", "-p", "--prompt", "-R", "--chroot", "-T", "--command-timeout", "-u", "--user", "-U", "--other-user"},
	"nice": {"-n", "--adjustment"},
	"exec": {"-a"},
}

type heredoc struct {
	delim     string
	quoted    bool // 終端語のどこかが引用されていれば、本文は展開されない
	stripTabs bool
}

func main() {
	// 出力失敗も含めて終了コードは 0 のまま。hint はツールの実行を妨げない。
	_ = run(os.Args[1:], os.Stdin, os.Stdout)
}

func run(args []string, r io.Reader, w io.Writer) error {
	if len(args) != 1 {
		return nil
	}
	var rules []rule
	if err := json.Unmarshal([]byte(args[0]), &rules); err != nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(r, maxInput+1))
	if err != nil || len(data) > maxInput {
		return nil
	}
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		return nil
	}
	if in.ToolName != "Bash" || in.ToolInput == nil || in.ToolInput.Command == nil {
		return nil
	}
	cmds, ok := commands(*in.ToolInput.Command, 0)
	if !ok {
		return nil
	}

	var hints []string
	for _, rl := range rules {
		re := patternRegexp(rl.Pattern)
		for _, c := range cmds {
			if re.MatchString(c) {
				hints = append(hints, hintPrefix+rl.Reason)
				break
			}
		}
	}
	if len(hints) == 0 {
		return nil
	}
	return json.MarshalWrite(w, map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":     "PreToolUse",
		"additionalContext": strings.Join(hints, "\n"),
	}})
}

// patternRegexp は permission rule の `*`（任意の文字列）だけを解釈し、行全体との一致を求める。
func patternRegexp(pattern string) *regexp.Regexp {
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`^(?s:` + strings.Join(parts, ".*") + `)$`)
}

// commands は s を単純コマンドに分け、それぞれを単語の空白区切りで返す。引用は外し、コマンド置換・
// バッククォート・プロセス置換・bash -c・eval の中身も調べる。リダイレクト先・case のパターン・配列の要素は
// コマンドとして扱わない。算術式と、終端語を引用していない heredoc の本文は、中のコマンド置換だけを調べる。
// 引用やコマンド置換が閉じていない場合は ok = false を返す。
//
// コマンド置換の閉じ括弧は括弧の数で探すので、中に case のパターン（`a)`）があると手前で閉じたとみなす。
func commands(s string, depth int) (cmds []string, ok bool) {
	if depth > maxDepth {
		return nil, true
	}
	var (
		words       []string
		word        strings.Builder
		inWord      bool
		quoted      bool
		nested      []string
		heredocs    []heredoc
		stripTabs   bool
		expectDelim bool
		skipWord    bool
		inArray     bool
		arraySep    int // 配列の中で、最後に要素の区切り（引用もエスケープもされていない空白か開き括弧）があった位置
		caseDepth   int
		inPattern   bool
	)
	endWord := func() {
		if !inWord {
			return
		}
		w, q := word.String(), quoted
		word.Reset()
		inWord, quoted = false, false
		switch {
		case expectDelim:
			heredocs = append(heredocs, heredoc{delim: w, quoted: q, stripTabs: stripTabs})
			expectDelim, stripTabs = false, false
		case skipWord:
			skipWord = false
		case caseDepth > 0 && len(words) == 0 && w == "esac" && !q:
			// 引用した "esac" はパターンで、予約語ではない
			caseDepth--
			inPattern = false
		default:
			words = append(words, w)
			if n := len(words); !inPattern && w == "in" && !q && n >= 3 && words[n-3] == "case" && onlyKeywords(words[:n-3]) {
				// case の対象の単語は展開されるがコマンドではない。中のコマンド置換は nested に入っている
				words = nil
				caseDepth++
				inPattern = true
			}
		}
	}
	endCmd := func() {
		endWord()
		skipWord = false
		if len(words) > 0 {
			sub, ok := simpleCommand(words, depth)
			if ok {
				cmds = append(cmds, sub...)
			}
			words = nil
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			if i+1 < len(s) {
				if s[i+1] != '\n' {
					word.WriteByte(s[i+1])
					inWord, quoted = true, true
				}
				i++
			}
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, false
			}
			word.WriteString(s[i+1 : i+1+j])
			inWord, quoted = true, true
			i += j + 1
		case c == '"':
			end, text, subs, ok := scanDouble(s, i+1)
			if !ok {
				return nil, false
			}
			word.WriteString(text)
			inWord, quoted = true, true
			nested = append(nested, subs...)
			i = end
		case c == '`':
			j := strings.IndexByte(s[i+1:], '`')
			if j < 0 {
				return nil, false
			}
			nested = append(nested, s[i+1:i+1+j])
			word.WriteString(s[i : i+2+j])
			inWord = true
			i += j + 1
		case c == '$' && i+1 < len(s) && s[i+1] == '(':
			end, subs, ok := dollarParen(s, i)
			if !ok {
				return nil, false
			}
			nested = append(nested, subs...)
			word.WriteString(s[i : end+1])
			inWord = true
			i = end
		case c == '#' && (!inWord || (inArray && arraySep == i-1)):
			for i+1 < len(s) && s[i+1] != '\n' {
				i++
			}
		case inArray && (c == ' ' || c == '\t' || c == '\r' || c == '\n'):
			// 配列の要素は代入の単語の一部として持ち、コマンドとして扱わない
			word.WriteByte(' ')
			arraySep = i
		case inArray && c == ')':
			word.WriteByte(c)
			inArray = false
		case c == ' ' || c == '\t' || c == '\r':
			endWord()
		case c == '\n':
			endCmd()
			if len(heredocs) > 0 {
				next, bodies := skipHeredocs(s, i+1, heredocs)
				for _, b := range bodies {
					_, _, subs, ok := scanExpansions(b, 0, false)
					if !ok {
						return nil, false
					}
					nested = append(nested, subs...)
				}
				i = next - 1
				heredocs = nil
			}
		case c == '<' && strings.HasPrefix(s[i:], "<<<"):
			endWord()
			i += 2
			skipWord = true
		case c == '<' && strings.HasPrefix(s[i:], "<<"):
			endWord()
			i++
			if i+1 < len(s) && s[i+1] == '-' {
				i++
				stripTabs = true
			}
			expectDelim = true
		case (c == '<' || c == '>') && i+1 < len(s) && s[i+1] == '(':
			// プロセス置換。中はコマンドとして読み、外側では引数の 1 単語として扱う
			end, ok := matchParen(s, i+2)
			if !ok {
				return nil, false
			}
			nested = append(nested, s[i+2:end])
			word.WriteString(s[i : end+1])
			inWord = true
			i = end
		case c == '<' || c == '>' || (c == '&' && i+1 < len(s) && s[i+1] == '>'):
			// リダイレクト。2>&1 の数字は直前の単語として残るが、照合には影響しない
			endWord()
			for i+1 < len(s) && (s[i+1] == '>' || s[i+1] == '&' || s[i+1] == '|') {
				i++
			}
			skipWord = true
		case inPattern && (c == '(' || c == '|'):
			// case のパターンの開き括弧と区切り
			endWord()
		case inPattern && c == ')':
			// case のパターンはコマンドではない
			endWord()
			words = nil
			inPattern = false
		case c == '(' && inWord && strings.HasSuffix(word.String(), "=") && isAssignment(word.String()):
			word.WriteByte(c)
			inArray = true
			arraySep = i
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			n := len(words)
			if n > 0 && words[n-1] == "for" {
				n--
			}
			if c == '(' && !inWord && onlyKeywords(words[:n]) {
				if end, subs, ok := doubleParen(s, i); ok {
					// (( 式 )) と for (( 式; 式; 式 ))。算術式の中のコマンド置換だけを調べる
					nested = append(nested, subs...)
					words = nil
					i = end
					continue
				}
			}
			if c == ';' && caseDepth > 0 && i+1 < len(s) && (s[i+1] == ';' || s[i+1] == '&') {
				// ;; / ;& / ;;& で case の節が終わり、次はパターン
				i++
				if s[i] == ';' && i+1 < len(s) && s[i+1] == '&' {
					i++
				}
				endCmd()
				inPattern = true
				continue
			}
			endCmd()
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	if inArray {
		return nil, false
	}
	endCmd()

	for _, n := range nested {
		sub, ok := commands(n, depth+1)
		if ok {
			cmds = append(cmds, sub...)
		}
	}
	return cmds, true
}

// simpleCommand は先頭の変数代入・予約語・ラッパーを外し、bash -c と eval は中身を解析する。
func simpleCommand(words []string, depth int) ([]string, bool) {
	for len(words) > 0 {
		w := words[0]
		switch {
		case isAssignment(w), keywords[w]:
			words = words[1:]
		case w == "command" && queriesCommand(words[1:]):
			// command -v / -V は対象を実行せず、説明を出すだけ（bash の man）
			return expand(words, depth)
		case wrappers[w]:
			words = stripWrapper(words)
		default:
			return expand(words, depth)
		}
	}
	return nil, true
}

func expand(words []string, depth int) ([]string, bool) {
	line := strings.Join(words, " ")
	switch words[0] {
	case "bash", "sh", "zsh":
		for i := 1; i+1 < len(words); i++ {
			if words[i] == "-c" {
				sub, ok := commands(words[i+1], depth+1)
				return append([]string{line}, sub...), ok
			}
		}
	case "eval":
		sub, ok := commands(strings.Join(words[1:], " "), depth+1)
		return append([]string{line}, sub...), ok
	}
	return []string{line}, true
}

// stripWrapper は words[0] のラッパーを、そのオプション（値を含む）と変数代入ごと外す。
func stripWrapper(words []string) []string {
	name := words[0]
	words = words[1:]
	for len(words) > 0 {
		w := words[0]
		switch {
		case w == "--":
			return words[1:]
		case isAssignment(w):
			words = words[1:]
		case !strings.HasPrefix(w, "-"):
			return words
		default:
			words = words[1:]
			opt, value, attached, ok := valueOption(name, w)
			if !ok {
				continue
			}
			if !attached && len(words) > 0 {
				value, words = words[0], words[1:]
			}
			if opt == "-S" || opt == "--split-string" {
				// env -S の値はコマンド行そのものなので、引用を解釈して分け、残りの単語の前に戻す
				fields, ok := splitWords(value)
				if !ok {
					fields = strings.Fields(value)
				}
				words = append(fields, words...)
			}
		}
	}
	return words
}

// valueOption は w がラッパーの値を取るオプションなら、その名前と、同じ単語に書かれた値を返す。
// 短いオプションはまとめて書けるので、値を取る文字の後ろが値になる（無ければ次の単語が値）。
func valueOption(wrapper, w string) (opt, value string, attached, ok bool) {
	values := wrapperValueOptions[wrapper]
	if strings.HasPrefix(w, "--") {
		opt, value, attached = strings.Cut(w, "=")
		return opt, value, attached, slices.Contains(values, opt)
	}
	for i := 1; i < len(w); i++ {
		if opt := "-" + w[i:i+1]; slices.Contains(values, opt) {
			return opt, w[i+1:], i+1 < len(w), true
		}
	}
	return "", "", false, false
}

// splitWords は s を単一引用符・二重引用符・バックスラッシュを解釈して単語に分ける。引用が閉じていなければ ok = false。
func splitWords(s string) (words []string, ok bool) {
	var (
		word   strings.Builder
		inWord bool
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case c == '\\' && i+1 < len(s):
			word.WriteByte(s[i+1])
			inWord = true
			i++
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, false
			}
			word.WriteString(s[i+1 : i+1+j])
			inWord = true
			i += j + 1
		case c == '"':
			end, text, _, ok := scanDouble(s, i+1)
			if !ok {
				return nil, false
			}
			word.WriteString(text)
			inWord = true
			i = end
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, true
}

// queriesCommand は command の引数のオプションに -v / -V があるか。
func queriesCommand(args []string) bool {
	for _, a := range args {
		if a == "--" || !strings.HasPrefix(a, "-") {
			return false
		}
		if strings.ContainsAny(a, "vV") {
			return true
		}
	}
	return false
}

func onlyKeywords(words []string) bool {
	for _, w := range words {
		if !keywords[w] {
			return false
		}
	}
	return true
}

func isAssignment(w string) bool {
	name, _, found := strings.Cut(w, "=")
	name = strings.TrimSuffix(name, "+")
	if !found || name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// dollarParen は s[i] から始まる "$(" を読み、閉じ括弧の位置と、コマンドとして解析する中身を返す。
// "$((式))" は算術展開なので、式の中のコマンド置換だけを返す。
func dollarParen(s string, i int) (end int, subs []string, ok bool) {
	if e, sub, ok := doubleParen(s, i+1); ok {
		return e, sub, true
	}
	end, ok = matchParen(s, i+2)
	if !ok {
		return 0, nil, false
	}
	return end, []string{s[i+2 : end]}, true
}

// doubleParen は s[i:] が "((" で始まって "))" で閉じる算術式なら、閉じ括弧の位置と式の中のコマンド置換を返す。
// "((cd x) && npm test)" のように内側の括弧が先に閉じるものはサブシェルなので ok = false。
func doubleParen(s string, i int) (end int, subs []string, ok bool) {
	if !strings.HasPrefix(s[i:], "((") {
		return 0, nil, false
	}
	inner, ok := matchParen(s, i+2)
	if !ok || inner+1 >= len(s) || s[inner+1] != ')' {
		return 0, nil, false
	}
	// 式は二重引用符の中と同じように展開されるが、二重引用符そのものは特別な意味を持たない（bash の man）
	_, _, subs, ok = scanExpansions(s[i+2:inner], 0, false)
	return inner + 1, subs, ok
}

// scanDouble は二重引用符の中を読み、閉じ引用符の位置・引用を外した文字列・中のコマンド置換を返す。
func scanDouble(s string, start int) (end int, text string, subs []string, ok bool) {
	return scanExpansions(s, start, true)
}

// scanExpansions は s[start:] を二重引用符の中と同じ規則で読む。quote が true なら閉じ引用符で止まり、
// false なら二重引用符も普通の文字として最後まで読む（算術式と、終端語を引用していない heredoc の本文）。
func scanExpansions(s string, start int, quote bool) (end int, text string, subs []string, ok bool) {
	var b strings.Builder
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' && quote:
			return i, b.String(), subs, true
		case c == '\\' && i+1 < len(s) && strings.IndexByte("$`\"\\\n", s[i+1]) >= 0:
			if s[i+1] != '\n' {
				b.WriteByte(s[i+1])
			}
			i++
		case c == '`':
			j := strings.IndexByte(s[i+1:], '`')
			if j < 0 {
				return 0, "", nil, false
			}
			subs = append(subs, s[i+1:i+1+j])
			b.WriteString(s[i : i+2+j])
			i += j + 1
		case c == '$' && i+1 < len(s) && s[i+1] == '(':
			e, sub, ok := dollarParen(s, i)
			if !ok {
				return 0, "", nil, false
			}
			subs = append(subs, sub...)
			b.WriteString(s[i : e+1])
			i = e
		default:
			b.WriteByte(c)
		}
	}
	if quote {
		return 0, "", nil, false
	}
	return len(s), b.String(), subs, true
}

// matchParen は start から読み、対応する ')' の位置を返す。中の引用とコメントは読み飛ばす。
func matchParen(s string, start int) (int, bool) {
	depth := 1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '#':
			// 単語の先頭の # からはコメント（$# や ${#x} や 16#ff の # は単語の途中）
			if i == start || strings.IndexByte(" \t\r\n;&|()", s[i-1]) >= 0 {
				for i+1 < len(s) && s[i+1] != '\n' {
					i++
				}
			}
		case '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return 0, false
			}
			i += j + 1
		case '"':
			end, _, _, ok := scanDouble(s, i+1)
			if !ok {
				return 0, false
			}
			i = end
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// skipHeredocs は start から heredoc の本文を読み飛ばし、最後の終端行の次の位置と、展開される本文
// （終端語を引用していないもの）を返す。
func skipHeredocs(s string, start int, docs []heredoc) (int, []string) {
	i := start
	var bodies []string
	for _, d := range docs {
		var body strings.Builder
		for i < len(s) {
			end := strings.IndexByte(s[i:], '\n')
			line := s[i:]
			next := len(s)
			if end >= 0 {
				line = s[i : i+end]
				next = i + end + 1
			}
			i = next
			if d.stripTabs {
				line = strings.TrimLeft(line, "\t")
			}
			if line == d.delim {
				break
			}
			body.WriteString(line)
			body.WriteByte('\n')
		}
		if !d.quoted {
			bodies = append(bodies, body.String())
		}
	}
	return i, bodies
}
