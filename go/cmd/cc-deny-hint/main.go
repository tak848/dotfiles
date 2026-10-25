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
// バッククォート・bash -c・eval の中身も調べる。heredoc の本文とリダイレクト先はコマンドとして扱わない。
// 引用やコマンド置換が閉じていない場合は ok = false を返す。
func commands(s string, depth int) (cmds []string, ok bool) {
	if depth > maxDepth {
		return nil, true
	}
	var (
		words       []string
		word        strings.Builder
		inWord      bool
		nested      []string
		heredocs    []string
		stripTabs   bool
		expectDelim bool
		skipWord    bool
	)
	endWord := func() {
		if !inWord {
			return
		}
		w := word.String()
		word.Reset()
		inWord = false
		switch {
		case expectDelim:
			heredocs = append(heredocs, w)
			expectDelim = false
		case skipWord:
			skipWord = false
		default:
			words = append(words, w)
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
					inWord = true
				}
				i++
			}
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, false
			}
			word.WriteString(s[i+1 : i+1+j])
			inWord = true
			i += j + 1
		case c == '"':
			end, text, subs, ok := scanDouble(s, i+1)
			if !ok {
				return nil, false
			}
			word.WriteString(text)
			inWord = true
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
			end, ok := matchParen(s, i+2)
			if !ok {
				return nil, false
			}
			nested = append(nested, s[i+2:end])
			word.WriteString(s[i : end+1])
			inWord = true
			i = end
		case c == '#' && !inWord:
			for i+1 < len(s) && s[i+1] != '\n' {
				i++
			}
		case c == ' ' || c == '\t' || c == '\r':
			endWord()
		case c == '\n':
			endCmd()
			if len(heredocs) > 0 {
				i = skipHeredocs(s, i+1, heredocs, stripTabs) - 1
				heredocs = nil
				stripTabs = false
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
		case c == '<' || c == '>' || (c == '&' && i+1 < len(s) && s[i+1] == '>'):
			// リダイレクト。2>&1 の数字は直前の単語として残るが、照合には影響しない。
			// <(...) / >(...) はプロセス置換で、中はコマンドとして読む
			endWord()
			for i+1 < len(s) && (s[i+1] == '>' || s[i+1] == '&' || s[i+1] == '|') {
				i++
			}
			skipWord = i+1 >= len(s) || s[i+1] != '('
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			endCmd()
		default:
			word.WriteByte(c)
			inWord = true
		}
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
		case isAssignment(w):
			words = words[1:]
		case w == "!" || w == "{" || w == "if" || w == "then" || w == "elif" || w == "else" ||
			w == "do" || w == "while" || w == "until" || w == "time":
			words = words[1:]
		case w == "command" || w == "builtin" || w == "exec" || w == "nohup" || w == "sudo" || w == "env" || w == "nice":
			words = words[1:]
			for len(words) > 0 && (strings.HasPrefix(words[0], "-") || isAssignment(words[0])) {
				words = words[1:]
			}
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

func isAssignment(w string) bool {
	name, _, found := strings.Cut(w, "=")
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

// scanDouble は二重引用符の中を読み、閉じ引用符の位置・引用を外した文字列・中のコマンド置換を返す。
func scanDouble(s string, start int) (end int, text string, subs []string, ok bool) {
	var b strings.Builder
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
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
			e, ok := matchParen(s, i+2)
			if !ok {
				return 0, "", nil, false
			}
			subs = append(subs, s[i+2:e])
			b.WriteString(s[i : e+1])
			i = e
		default:
			b.WriteByte(c)
		}
	}
	return 0, "", nil, false
}

// matchParen は start から読み、対応する ')' の位置を返す。中の引用は読み飛ばす。
func matchParen(s string, start int) (int, bool) {
	depth := 1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
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

// skipHeredocs は start から heredoc の本文を読み飛ばし、最後の終端行の次の位置を返す。
func skipHeredocs(s string, start int, delims []string, stripTabs bool) int {
	i := start
	for _, d := range delims {
		for i < len(s) {
			end := strings.IndexByte(s[i:], '\n')
			line := s[i:]
			next := len(s)
			if end >= 0 {
				line = s[i : i+end]
				next = i + end + 1
			}
			i = next
			if stripTabs {
				line = strings.TrimLeft(line, "\t")
			}
			if line == d {
				break
			}
		}
	}
	return i
}
