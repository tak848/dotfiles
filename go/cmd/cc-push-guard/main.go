// cc-push-guard は Bash のリテラルな git push をツール実行前に検査する。
// 完全なシェル解析・外部 wrapper・MCP 等の別経路・検査後の状態変更（TOCTOU）は保証範囲外。
package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tak848/dotfiles/go/internal/gitstate"
	"github.com/tak848/dotfiles/go/internal/render"
)

const denySyntax = "push の対象を安全に確認できません。動的展開・eval・ブランチ変更などを分け、リテラルの git push 単独コマンドで再確認してください。"

type token struct {
	text              string
	operator, dynamic bool
}

// lex はシェルを実行しない。対応するリテラル形式の判定に必要な引用・エスケープ・演算子を保持する。
func lex(s string) ([]token, error) {
	var tokens []token
	var word strings.Builder
	started, dynamic := false, false
	quote := byte(0)
	flush := func() {
		if started {
			tokens = append(tokens, token{text: word.String(), dynamic: dynamic})
			word.Reset()
			started = false
			dynamic = false
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote == '`' {
			word.WriteByte(ch)
			if ch == '`' {
				quote = 0
			}
			continue
		}
		if quote == '\'' {
			if ch == '\'' {
				quote = 0
			} else {
				word.WriteByte(ch)
			}
			continue
		}
		if ch == '\\' {
			started = true
			if i+1 >= len(s) {
				return tokens, fmt.Errorf("escape")
			}
			next := s[i+1]
			if quote == '"' && next != '$' && next != '`' && next != '"' && next != '\\' && next != '\n' {
				word.WriteByte(ch)
				continue
			}
			i++
			if next != '\n' {
				word.WriteByte(next)
			}
			continue
		}
		if quote == '"' {
			if ch == '"' {
				quote = 0
			} else {
				if ch == '$' || ch == '`' {
					dynamic = true
				}
				word.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '\'', '"':
			started = true
			quote = ch
		case '`':
			started = true
			dynamic = true
			quote = ch
			word.WriteByte(ch)
		case '$', '~', '*', '?', '[', '{':
			started = true
			dynamic = true
			word.WriteByte(ch)
		case ' ', '\t', '\r':
			flush()
		case '#':
			if !started {
				for i < len(s) && s[i] != '\n' {
					i++
				}
				if i < len(s) {
					tokens = append(tokens, token{text: "\n", operator: true})
				}
			} else {
				word.WriteByte(ch)
			}
		case '&', '|', ';', '\n', '(', ')', '<', '>':
			flush()
			op := string(ch)
			if i+1 < len(s) && s[i+1] == ch && (ch == '&' || ch == '|' || ch == '<' || ch == '>') {
				op += string(ch)
				i++
			}
			tokens = append(tokens, token{text: op, operator: true})
		default:
			started = true
			word.WriteByte(ch)
		}
	}
	flush()
	if quote != 0 {
		return tokens, fmt.Errorf("quote")
	}
	return tokens, nil
}

func potentialPush(ts []token, depth int) bool {
	if depth > 4 {
		return true
	}
	// 実行されるコマンド置換を調べる。引用された説明文字列は実行扱いしない。
	for _, t := range ts {
		if t.dynamic && (strings.Contains(t.text, "$(") || strings.Contains(t.text, "`")) {
			nested, _ := lex(strings.NewReplacer("$(", " ; ", "`", " ; ").Replace(t.text))
			if potentialPush(nested, depth+1) {
				return true
			}
		}
	}
	for start := 0; start < len(ts); {
		end := start
		for end < len(ts) && !ts[end].operator {
			end++
		}
		if commandPush(ts[start:end], depth) {
			return true
		}
		start = end + 1
	}
	return false
}
func commandPush(ts []token, depth int) bool {
	for len(ts) > 0 && strings.Contains(ts[0].text, "=") {
		ts = ts[1:]
	}
	if len(ts) == 0 {
		return false
	}
	name := filepath.Base(ts[0].text)
	if ts[0].dynamic && len(ts) > 1 && ts[1].text == "push" {
		return true
	}
	switch name {
	case "then", "do", "else", "elif", "if", "while", "until", "!", "{":
		// 予約語の直後はコマンド位置。複合構文は検知だけ行い、parsePush 側で拒否する。
		return commandPush(ts[1:], depth+1)
	case "function":
		for i, a := range ts[1:] {
			if a.text == "{" {
				return commandPush(ts[i+2:], depth+1)
			}
		}
		return false
	case "git":
		for i := 1; i < len(ts); i++ {
			a := ts[i]
			if a.dynamic {
				return true
			}
			switch a.text {
			case "-C", "-c", "--git-dir", "--work-tree", "--namespace":
				i++
				continue
			}
			if strings.HasPrefix(a.text, "-") {
				continue
			}
			return a.text == "push"
		}
		return false
	case "command", "builtin", "exec", "env", "sudo", "nohup", "time", "nice":
		for i := 1; i < len(ts); i++ {
			if filepath.Base(ts[i].text) == "git" && commandPush(ts[i:], depth+1) {
				return true
			}
		}
		return false
	case "eval":
		var parts []string
		for _, a := range ts[1:] {
			parts = append(parts, a.text)
		}
		nested, _ := lex(strings.Join(parts, " "))
		return potentialPush(nested, depth+1)
	case "bash", "sh", "zsh":
		for _, a := range ts[1:] {
			nested, _ := lex(a.text)
			if potentialPush(nested, depth+1) {
				return true
			}
		}
	}
	return false
}

// parsePush は echo 'git push' など無関係なコマンドを対象外とする。未対応の push 構文は拒否する。
func parsePush(cwd, command string) (dir string, args []string, applicable bool, err error) {
	ts, lexErr := lex(strings.TrimSpace(command))
	if !potentialPush(ts, 0) {
		return "", nil, false, nil
	}
	fail := func() (string, []string, bool, error) { return "", nil, true, fmt.Errorf("%s", denySyntax) }
	if lexErr != nil {
		return fail()
	}
	for _, t := range ts {
		if t.dynamic {
			return fail()
		}
	}
	dir = cwd
	if len(ts) >= 4 && ts[0].text == "cd" && !ts[0].operator {
		p := 1
		if ts[p].text == "--" {
			p++
		}
		if len(ts) <= p+2 || ts[p].operator || ts[p+1].text != "&&" {
			return fail()
		}
		if strings.HasPrefix(ts[p].text, "-") || ts[p].text == "" {
			return fail()
		}
		dir = joinLogicalDir(dir, ts[p].text)
		ts = ts[p+2:]
	}
	if len(ts) == 0 || ts[0].text != "git" {
		return fail()
	}
	i := 1
	for i < len(ts) && ts[i].text == "-C" {
		if i+1 >= len(ts) || ts[i+1].operator || ts[i+1].text == "" {
			return fail()
		}
		dir = appendPhysicalDir(dir, ts[i+1].text)
		i += 2
	}
	if i >= len(ts) || ts[i].text != "push" {
		return fail()
	}
	i++
	for _, t := range ts[i:] {
		if t.operator {
			return fail()
		}
		args = append(args, t.text)
	}
	return dir, args, true, nil
}

// git -C は chdir の物理解決を使う。symlink/.. を事前に Clean すると別リポジトリを検査するので、生のパスを exec.Dir まで保持する。
func appendPhysicalDir(cwd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return strings.TrimSuffix(cwd, "/") + "/" + path
}

// 通常の cd はシェルの論理解決。git -C の物理解決と共用しない。
func joinLogicalDir(cwd, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(cwd, path)
}

type input struct {
	CWD       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput *struct {
		Command *string `json:"command"`
	} `json:"tool_input"`
}
type checkFunc func(context.Context, string, []string) error

func run(ctx context.Context, r io.Reader, w io.Writer, check checkFunc) error {
	return runWithEnv(ctx, r, w, check, os.Getenv)
}
func runWithEnv(ctx context.Context, r io.Reader, w io.Writer, check checkFunc, getenv func(string) string) error {
	switch strings.ToLower(strings.TrimSpace(getenv("CC_PUSH_GUARD"))) {
	case "0", "false", "off", "no":
		return nil
	}
	// 壊れた入力を無関係なコマンドとして黙って許可しない。
	data, e := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if e != nil {
		return e
	}
	if len(data) > 1<<20 {
		return writeDeny(w, "push guard の入力が上限を超えました。単独コマンドに分けてください。")
	}
	var in input
	if e = json.Unmarshal(data, &in); e != nil {
		return writeDeny(w, "push guard の入力を読み取れません。hook 入力を確認してください。")
	}
	if in.ToolName == "" {
		return writeDeny(w, "push guard の tool_name がありません。hook 入力を確認してください。")
	}
	if in.ToolName != "Bash" {
		return nil
	}
	if in.ToolInput == nil || in.ToolInput.Command == nil {
		return writeDeny(w, "push guard の command がありません。hook 入力を確認してください。")
	}
	dir, args, applies, e := parsePush(in.CWD, *in.ToolInput.Command)
	if !applies {
		return nil
	}
	if e == nil {
		if dir == "" || !filepath.IsAbs(dir) {
			e = fmt.Errorf("%s", denySyntax)
		} else {
			e = check(ctx, dir, args)
		}
	}
	if e != nil {
		reason := e.Error()
		if dir != "" {
			reason += "\n作業ディレクトリ: " + render.Truncate(render.Sanitize(dir), 300)
		}
		return writeDeny(w, reason)
	}
	return nil
}
func writeDeny(w io.Writer, reason string) error {
	return json.MarshalWrite(w, map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": reason}})
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), gitstate.PushTimeout)
	defer cancel()
	if err := run(ctx, os.Stdin, os.Stdout, gitstate.CheckPush); err != nil {
		fmt.Fprintln(os.Stderr, "push guard の入出力に失敗しました。")
		os.Exit(2)
	}
}
