// cc-push-guard は Bash のコマンドに含まれる git push を、ツール実行前に検査する。
// 外部 wrapper・MCP 等の別経路・検査後の状態変更（TOCTOU）は保証範囲外。
package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/tak848/dotfiles/go/internal/gitstate"
	"github.com/tak848/dotfiles/go/internal/render"
)

const denyUnverifiable = "このコマンドの git push は push 先を確かめられない形（remote やブランチ名・cd 先の変数やコマンド置換、git の -c などの大域オプション、GIT_ で始まる環境変数、展開されるコマンド名）なので、実行しませんでした。remote とブランチ名をリテラルで書いて実行し直してください。ほかのコマンドとの連結やパイプはそのままでかまいません。"

var errUnverifiable = errors.New(denyUnverifiable)

// maxDepth は bash -c / eval の中身を解析し直す深さの上限。
const maxDepth = 4

// push は検査する git push の作業ディレクトリと、push に渡す引数。dir が空なら作業ディレクトリが分からない。
type push struct {
	dir  string
	args []string
}

// word はコマンドの単語。literal は展開（変数・コマンド置換・glob・チルダなど）を含まないこと。
type word struct {
	text    string
	literal bool
}

// findPushes は command をシェルの構文として解析し、実行される git push を出てくる順に返す。連結・パイプ・
// リダイレクト・コマンド置換・複合コマンドの中の push も拾い、引用された文字列や heredoc の本文はコマンドに
// しない。bash -c と eval の中身は改めて解析する。リテラルな cd は出てきた順に作業ディレクトリへ反映する。
// push 先を確かめられない push があれば errUnverifiable を返す。解析できない command は push 無しとして扱う。
func findPushes(dir, command string, depth int) ([]push, error) {
	if depth > maxDepth {
		return nil, nil
	}
	file, err := parse(command)
	if err != nil {
		return nil, nil
	}
	var found []push
	var failed error
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if failed != nil || !ok || len(call.Args) == 0 {
			return failed == nil
		}
		gitEnv := false
		for _, a := range call.Assigns {
			if a.Name != nil && strings.HasPrefix(a.Name.Value, "GIT_") {
				gitEnv = true
			}
		}
		ws := make([]word, len(call.Args))
		for i, w := range call.Args {
			ws[i] = wordOf(w)
		}
		ws, wrapperGitEnv, unknown := stripWrappers(ws)
		gitEnv = gitEnv || wrapperGitEnv
		switch {
		case unknown:
			failed = errUnverifiable
			return false
		case len(ws) == 0:
			return true
		case !ws[0].literal:
			// コマンド名が展開される（$GIT push など）
			if len(ws) > 1 && ws[1].text == "push" {
				failed = errUnverifiable
			}
			return failed == nil
		}
		switch filepath.Base(ws[0].text) {
		case "cd":
			dir = cdTarget(dir, ws[1:])
		case "bash", "sh", "zsh":
			for i := 1; i+1 < len(ws); i++ {
				if ws[i].text == "-c" {
					sub, e := findPushes(dir, ws[i+1].text, depth+1)
					found, failed = append(found, sub...), e
					break
				}
			}
		case "eval":
			texts := make([]string, 0, len(ws)-1)
			for _, w := range ws[1:] {
				texts = append(texts, w.text)
			}
			sub, e := findPushes(dir, strings.Join(texts, " "), depth+1)
			found, failed = append(found, sub...), e
		case "git":
			p, isPush, e := gitPush(dir, ws[1:])
			if isPush && gitEnv {
				e = errUnverifiable
			}
			if isPush && e == nil {
				found = append(found, p)
			}
			failed = e
		}
		return failed == nil
	})
	return found, failed
}

// gitPush は git の引数（git 自身は除く）が push なら、その作業ディレクトリと push の引数を返す。
func gitPush(dir string, ws []word) (push, bool, error) {
	verifiable := true
	for i := 0; i < len(ws); i++ {
		w := ws[i]
		switch {
		case !w.literal:
			// サブコマンドの位置が展開されるので、push かどうかも分からない
			return push{}, true, errUnverifiable
		case w.text == "-C":
			if i+1 >= len(ws) {
				return push{}, false, nil
			}
			if v := ws[i+1]; v.literal && v.text != "" && dir != "" {
				dir = appendPhysicalDir(dir, v.text)
			} else {
				dir = ""
			}
			i++
		case w.text == "-c" || w.text == "--git-dir" || w.text == "--work-tree" || w.text == "--namespace":
			verifiable = false
			i++
		case strings.HasPrefix(w.text, "-"):
			if strings.HasPrefix(w.text, "--git-dir=") || strings.HasPrefix(w.text, "--work-tree=") || strings.HasPrefix(w.text, "--namespace=") {
				verifiable = false
			}
		case w.text != "push":
			return push{}, false, nil
		default:
			args := make([]string, 0, len(ws)-i-1)
			for _, a := range ws[i+1:] {
				verifiable = verifiable && a.literal
				args = append(args, a.text)
			}
			if !verifiable || dir == "" {
				return push{}, true, errUnverifiable
			}
			return push{dir: dir, args: args}, true, nil
		}
	}
	return push{}, false, nil
}

// cdTarget は cd の引数から移動先を返す。リテラルでない・オプション付きの cd の後は作業ディレクトリが分からない。
func cdTarget(dir string, args []word) string {
	if len(args) > 0 && args[0].literal && args[0].text == "--" {
		args = args[1:]
	}
	if dir == "" || len(args) != 1 || !args[0].literal || args[0].text == "" || strings.HasPrefix(args[0].text, "-") {
		return ""
	}
	return joinLogicalDir(dir, args[0].text)
}

// stripWrappers は先頭のラッパー（command・builtin・exec・nohup・time・env）を外す。env の変数代入も外し、
// GIT_ で始まる変数を代入していれば gitEnv を返す。command -v / -V は対象を実行しないので空にする。
// env にオプションが付いていて、その後に git push らしい並びがあれば unknown を返す（値を取るオプションが
// あり、どこからがコマンドか決められない）。
func stripWrappers(ws []word) (rest []word, gitEnv, unknown bool) {
	for len(ws) > 0 && ws[0].literal {
		switch ws[0].text {
		case "command":
			if len(ws) > 1 && (ws[1].text == "-v" || ws[1].text == "-V") {
				return nil, gitEnv, false
			}
			ws = ws[1:]
			if len(ws) > 0 && ws[0].text == "-p" {
				ws = ws[1:]
			}
		case "builtin", "exec", "nohup", "time":
			ws = ws[1:]
		case "env":
			ws = ws[1:]
			for len(ws) > 0 && isAssignment(ws[0].text) {
				gitEnv = gitEnv || strings.HasPrefix(ws[0].text, "GIT_")
				ws = ws[1:]
			}
			if len(ws) > 0 && strings.HasPrefix(ws[0].text, "-") {
				return nil, gitEnv, mentionsPush(ws)
			}
		default:
			return ws, gitEnv, false
		}
	}
	return ws, gitEnv, false
}

// mentionsPush は単語の並びに git とその後の push（または展開される単語）が現れるかを返す。
func mentionsPush(ws []word) bool {
	git := false
	for _, w := range ws {
		switch {
		case git && (!w.literal || w.text == "push"):
			return true
		case filepath.Base(w.text) == "git":
			git = true
		}
	}
	return false
}

func isAssignment(w string) bool {
	name, _, ok := strings.Cut(w, "=")
	if !ok || name == "" {
		return false
	}
	for i, c := range name {
		if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
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

// wordOf は単語の引用とエスケープを外した文字列を返す。展開は書かれたまま残し、literal を false にする。
// 引用されていない glob の文字と先頭のチルダも展開として扱う。
func wordOf(w *syntax.Word) word {
	var b strings.Builder
	literal := true
	for i, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.ContainsAny(p.Value, "*?[{") || (i == 0 && strings.HasPrefix(p.Value, "~")) {
				literal = false
			}
			b.WriteString(unescape(p.Value, false))
		case *syntax.SglQuoted:
			literal = literal && !p.Dollar
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				if lit, ok := q.(*syntax.Lit); ok {
					b.WriteString(unescape(lit.Value, true))
				} else {
					literal = false
					_ = syntax.NewPrinter().Print(&b, q)
				}
			}
		default:
			literal = false
			_ = syntax.NewPrinter().Print(&b, part)
		}
	}
	return word{text: b.String(), literal: literal}
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
	// 確認できない場合は無出力で通常の permission 判定へ戻す。明示的な allow は返さない。
	data, e := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if e != nil || len(data) > 1<<20 {
		return nil
	}
	var in input
	if e = json.Unmarshal(data, &in); e != nil {
		return nil
	}
	if in.ToolName != "Bash" || in.ToolInput == nil || in.ToolInput.Command == nil || !filepath.IsAbs(in.CWD) {
		return nil
	}
	pushes, e := findPushes(in.CWD, *in.ToolInput.Command, 0)
	// push 先を確かめられない push を通すと、削除済みのマージ済みブランチを作り直せてしまうので止める。
	if e != nil {
		return writeDeny(w, denyUnverifiable)
	}
	for _, p := range pushes {
		if ctx.Err() != nil || !filepath.IsAbs(p.dir) {
			return nil
		}
		e = check(ctx, p.dir, p.args)
		if ctx.Err() != nil {
			return nil
		}
		if !errors.Is(e, gitstate.ErrMergedBranch) {
			continue
		}
		// 具体的な内容は型付きの error からだけ取り出す。任意の error 文字列は返さない。
		text := gitstate.ErrMergedBranch.Error()
		if merged, ok := errors.AsType[*gitstate.MergedBranchError](e); ok {
			text = render.Sanitize(merged.Error())
		}
		reason := text + "\n作業ディレクトリ: " + render.Truncate(render.Sanitize(p.dir), 300)
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
	// 出力失敗も exit 2 に変換せず、通常のツール実行を妨げない。
	_ = run(ctx, os.Stdin, os.Stdout, gitstate.CheckPush)
}
