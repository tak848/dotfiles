// cc-stop-gate は、応答終了時に plan / 依頼の完遂確認を要求する。
// 編集履歴、内部 TODO、transcript の読み取りには依存しない。
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tak848/dotfiles/go/internal/gitstate"
)

const (
	maxInputBytes  = 4 << 20
	maxReasonBytes = 9000
	checkTimeout   = 25 * time.Second
)

type input struct {
	Event           string           `json:"hook_event_name"`
	Mode            string           `json:"permission_mode"`
	CWD             string           `json:"cwd"`
	LastMessage     *string          `json:"last_assistant_message"`
	BackgroundTasks []jsontext.Value `json:"background_tasks"`
	SessionCrons    []jsontext.Value `json:"session_crons"`
}

type decision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type checkFunc func(context.Context, string, []string) []string

type lookupEnv func(string) (string, bool)

func main() {
	code := run(os.Stdin, os.Stdout, os.LookupEnv, gitstate.CheckStop)
	if code != 0 {
		fmt.Fprintln(os.Stderr, "作業結果の確認内容を出力できませんでした。出力先の確認が必要です。")
	}
	os.Exit(code)
}

func run(stdin io.Reader, stdout io.Writer, env lookupEnv, check checkFunc) int {
	value, _ := env("CC_STOP_GATE")
	if disabled(value) {
		return 0
	}

	data, err := io.ReadAll(io.LimitReader(stdin, maxInputBytes+1))
	if err != nil || len(data) > maxInputBytes {
		return emit(stdout, block("作業結果の確認に必要なデータを読み取れません。実行設定の確認が必要です。"))
	}
	var in *input
	if err := json.Unmarshal(data, &in); err != nil || in == nil {
		return emit(stdout, block("作業結果の確認に必要なデータ形式が不正です。実行設定の確認が必要です。"))
	}
	if in.Event != "Stop" {
		return emit(stdout, block("作業結果を確認するタイミングの設定が不正です。実行設定の確認が必要です。"))
	}
	if in.LastMessage == nil {
		return emit(stdout, block("現在の作業結果の報告文を取得できません。作業結果を報告してください。"))
	}
	switch in.Mode {
	case "plan", "default", "acceptEdits", "auto", "dontAsk", "bypassPermissions":
	default:
		return emit(stdout, block("計画中か実装中かを確認できません。誤った Git 操作を要求しないため、現在の実行モードの設定を確認してください。"))
	}

	message := analyze(*in.LastMessage)
	f := facts{
		CWD:      in.CWD,
		Mode:     in.Mode,
		Message:  message,
		Inflight: inflightCount(in.BackgroundTasks) + inflightCount(in.SessionCrons),
	}
	// 待機と plan mode は git より先。背景作業中や計画作成中の差分に
	// commit を要求せず、不要なネットワーク照会も行わない。
	if message.Signature != waiting && in.Mode != "plan" {
		if !filepath.IsAbs(in.CWD) {
			f.Issues = []string{gitstate.CheckFailurePrefix + " [input-cwd] 作業ディレクトリを特定できません。対象ディレクトリの設定を確認してください。"}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
			f.Issues = check(ctx, in.CWD, requiredPROwners(env))
			if ctx.Err() != nil {
				f.Issues = append(f.Issues, gitstate.CheckFailurePrefix+" [deadline] git / PR の検査時間を超過しました。plan の照合では直らないため、先に表示された照会の接続・実行時間を確認してください。")
			}
			cancel()
		}
	}
	return emit(stdout, decide(f))
}

// 要素が null / 空オブジェクトの壊れた入力を稼働中の仕事として数えない。
// 実行中の仕事の抽出自体は Claude Code の registry が担う。
func inflightCount(items []jsontext.Value) int {
	count := 0
	for _, raw := range items {
		var item struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &item) == nil && strings.TrimSpace(item.ID) != "" {
			count++
		}
	}
	return count
}

func disabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "off", "no":
		return true
	default:
		return false
	}
}

func requiredPROwners(env lookupEnv) []string {
	value, set := env("CC_STOP_GATE_REQUIRE_PR_OWNERS")
	if !set {
		return []string{"tak848"}
	}
	var owners []string
	seen := map[string]bool{}
	for _, owner := range strings.Split(value, ",") {
		owner = strings.ToLower(strings.TrimSpace(owner))
		if owner != "" && !seen[owner] {
			owners = append(owners, owner)
			seen[owner] = true
		}
	}
	return owners
}

func emit(w io.Writer, d decision) int {
	if d.Decision == "" {
		return 0
	}
	if err := json.MarshalWrite(w, d); err != nil {
		return 2
	}
	return 0
}

func block(reason string) decision {
	return decision{Decision: "block", Reason: clip(reason, maxReasonBytes)}
}

// byte 上限を守りつつ UTF-8 を途中で切らない。動的な理由を先に制限して、
// 最後に付ける完遂確認の指示が切り落とされないようにする。
func clip(text string, limit int) string {
	text = strings.ToValidUTF8(text, "?")
	if len(text) <= limit {
		return text
	}
	end := limit - len("…")
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}
