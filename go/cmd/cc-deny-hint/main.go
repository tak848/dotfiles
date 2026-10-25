// cc-deny-hint は permissions.deny に当たりうる Bash コマンドに、その理由を additionalContext で添える
// PreToolUse hook。ルールは引数の JSON（dot_claude/permission-rules.libsonnet が生成）で受け取る。
//
// hooks の `if` で同じことをすると、コマンド名より先まで指定したパターンは `$()`・バッククォート・`$VAR` を
// 含むだけで常に hook が実行される（公式 docs の hooks "How if patterns match Bash commands"）。そのため
// 無関係なコマンドにも全ルールの理由が注入されていた。ここではコマンドを構文解析して実行される単純コマンドを
// 集めて照合し、当たったルールの理由だけを返す。実際の拒否は permissions.deny が行うので、解析できないときは
// 何も返さない。
package main

import (
	"encoding/json/v2"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
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

// commands は s をシェルの構文として解析し、実行される単純コマンドをそれぞれ単語の空白区切りで返す。
// 構文木を辿るので、引用・コメント・heredoc・case のパターン・算術式・配列の要素・リダイレクト先はコマンドに
// ならず、コマンド置換・プロセス置換・展開される heredoc の本文の中は調べる。bash -c と eval の中身は、
// 文字列として改めて解析する。解析できなければ ok = false を返す。
func commands(s string, depth int) (cmds []string, ok bool) {
	if depth > maxDepth {
		return nil, true
	}
	file, err := parse(s)
	if err != nil {
		return nil, false
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		call, isCall := node.(*syntax.CallExpr)
		if !isCall || len(call.Args) == 0 {
			return true
		}
		words := make([]string, len(call.Args))
		for i, w := range call.Args {
			words[i] = wordString(w)
		}
		if sub, ok := simpleCommand(words, depth); ok {
			cmds = append(cmds, sub...)
		}
		return true
	})
	return cmds, true
}

// parse は s を bash として解析し、だめなら zsh として解析する。Bash ツールのコマンドはユーザーのシェルで
// 実行されるので、zsh の構文が来ることがある（mvdan.cc/sh の zsh 対応は実験的なので、予備にとどめる）。
func parse(s string) (*syntax.File, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(s), "")
	if err == nil {
		return file, nil
	}
	return syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(s), "")
}

// wordString は単語の引用とエスケープを外した文字列を返す。展開（$VAR・$(…)・$((…)) など）は書かれたまま残す。
func wordString(w *syntax.Word) string {
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(unescape(p.Value, false))
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				if lit, ok := q.(*syntax.Lit); ok {
					b.WriteString(unescape(lit.Value, true))
				} else {
					_ = syntax.NewPrinter().Print(&b, q)
				}
			}
		default:
			_ = syntax.NewPrinter().Print(&b, part)
		}
	}
	return b.String()
}

// unescape はバックスラッシュによるエスケープを外す。行継続は取り除く。二重引用符の中では、
// バックスラッシュは $ ` " \ と改行の前でだけ特別な意味を持つ（bash の man の QUOTING）。
func unescape(s string, inDouble bool) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) || (inDouble && strings.IndexByte("$`\"\\\n", s[i+1]) < 0) {
			b.WriteByte(s[i])
			continue
		}
		i++
		if s[i] != '\n' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// simpleCommand は先頭のラッパーを外し、bash -c と eval は中身を解析する。
func simpleCommand(words []string, depth int) ([]string, bool) {
	for len(words) > 0 {
		w := words[0]
		switch {
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
			words = words[1:]
			// env はオプションの後に NAME=VALUE を取る（[OPTION]... [-] [NAME=VALUE]... [COMMAND [ARG]...]）
			for name == "env" && len(words) > 0 && isAssignment(words[0]) {
				words = words[1:]
			}
			return words
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
				fields, ok := splitEnvString(value)
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

// envEscapes は env -S の値で使えるエスケープ（GNU coreutils のマニュアルの "-S/--split-string syntax"）。
// \_ と \c は位置によって意味が変わるので splitEnvString で扱う。
var envEscapes = map[byte]byte{
	'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v', '#': '#', '$': '$', '"': '"', '\'': '\'', '\\': '\\',
}

// splitEnvString は env -S の値を、GNU env の規則で単語に分ける。空白（タブ・改行なども含む）と、引用の外の
// \_ が区切りになる。単一引用符の中で使えるエスケープは \' と \\ だけ。引用の外で単語の先頭の # から後と、
// \c から後は無視される。引用が閉じていない、または知らないエスケープがあれば ok = false を返す。
func splitEnvString(s string) (words []string, ok bool) {
	var (
		word   strings.Builder
		inWord bool
		quote  byte
	)
	endWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == 0 && strings.IndexByte(" \t\n\r\v\f", c) >= 0:
			endWord()
		case quote == 0 && c == '#' && !inWord:
			return words, true
		case c == quote:
			quote = 0
		case quote == 0 && (c == '\'' || c == '"'):
			quote = c
			inWord = true
		case c == '\\' && i+1 < len(s):
			i++
			e := s[i]
			switch {
			case quote == '\'' && e != '\'' && e != '\\':
				word.WriteByte('\\')
				word.WriteByte(e)
			case e == '_' && quote == 0:
				endWord()
			case e == '_':
				word.WriteByte(' ')
			case e == 'c' && quote == 0:
				endWord()
				return words, true
			case envEscapes[e] != 0:
				word.WriteByte(envEscapes[e])
			default:
				return nil, false
			}
			if e != '_' || quote != 0 {
				inWord = true
			}
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	endWord()
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
