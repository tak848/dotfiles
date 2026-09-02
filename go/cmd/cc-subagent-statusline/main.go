// cc-subagent-statusline は Claude Code の subagentStatusLine コマンド。
// subagent 実行中にプロンプト下へ出るパネルの各行を差し替える。既定の
// 「name · description · token count」に代えて、モデル・effort・コンテキスト
// 使用率を出す。
//
// stdin には表示中の行がまとめて 1 つの JSON で渡り、stdout には
// {"id": ..., "content": ...} を 1 行ずつ返す。id を出さなかった行は既定描画の
// まま残り、content が空文字の行は消える。
//
// 壊れた出力は Claude Code 側で警告になるため、迷ったら無出力にする。
// cc-statusline とは逆に、ここでは無出力が「既定描画へのフォールバック」という
// 安全側に倒れる。
package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/tak848/dotfiles/go/internal/colors"
	"github.com/tak848/dotfiles/go/internal/render"
)

// placeholder は値がまだ届いていない箇所を埋める。モデルが解決する前や最初の
// トークン数が入る前に枠ごと消すと、直後に行が伸びて表示が飛ぶ。
const placeholder = "--"

// task は subagent パネルの 1 行。model と contextWindowSize は v2.1.205 以降、
// effort は v2.1.214 以降で、いずれも欠落しうる。
type task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Label       string `json:"label"`
	Model       string `json:"model"`
	// Effort はレベル文字列か数値のトークン予算のどちらかで来る。型を固定すると
	// 1 タスクの型違いで全行が既定描画に戻るため、生のまま受けて自前で解く。
	Effort            json.RawMessage `json:"effort"`
	ContextWindowSize float64         `json:"contextWindowSize"`
	TokenCount        float64         `json:"tokenCount"`
}

type input struct {
	Columns float64 `json:"columns"`
	Tasks   []task  `json:"tasks"`
}

type row struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// statusColor は実行状態に対応する色を返す。未知の状態は色を付けない。
func statusColor(status string) string {
	switch status {
	case "running", "in_progress", "active":
		return colors.Teal
	case "completed", "done", "success":
		return colors.Green
	case "failed", "error", "cancelled":
		return colors.Red
	case "pending", "queued", "waiting":
		return colors.Yellow
	default:
		return ""
	}
}

// effortLevel は effort をそのまま返す。数値のトークン予算で来た場合は表示
// しない（桁が大きく、行幅に見合わないため）。
func effortLevel(raw json.RawMessage) string {
	var level string
	if err := json.Unmarshal(raw, &level); err != nil {
		return ""
	}
	return render.Sanitize(level)
}

// renderTask は 1 行の本体を組み立てる。content が空だと行そのものが消えるので、
// 何も情報が無い場合でも最低限の識別子を返す。モデルや使用率がまだ解決して
// いないタスクでも枠を出し、値が届いた瞬間に行が伸びないようにする。
func renderTask(t task, width int) string {
	badge := render.ShortModel(t.Model)
	if badge == "" {
		badge = placeholder
	}
	if e := effortLevel(t.Effort); e != "" {
		badge += "·" + e
	}
	segs := []render.Segment{render.Seg(colors.Surface+badge+colors.Reset, 3)}

	pct := -1
	if t.ContextWindowSize > 0 && t.TokenCount > 0 {
		pct = max(0, min(int(t.TokenCount/t.ContextWindowSize*100), 100))
	}
	pctText := colors.Surface + placeholder + "%" + colors.Reset
	if pct >= 0 {
		pctText = render.UsageColor(pct) + strconv.Itoa(pct) + "%" + colors.Reset
	}
	segs = append(segs,
		render.Seg(render.Bar(max(pct, 0), 5), 2),
		render.Seg(pctText, 1),
	)

	name := render.Sanitize(t.Name)
	if name == "" {
		name = render.Sanitize(t.Type)
	}
	if name == "" {
		name = "task"
	}
	segs = append(segs, render.Seg(statusColor(t.Status)+name+colors.Reset, 0))

	// label は Claude Code 側で description から導かれた表示用の文字列で、
	// 無い場合だけ description に落とす。
	detail := render.Sanitize(t.Label)
	if detail == "" {
		detail = render.Sanitize(t.Description)
	}
	if detail != "" {
		segs = append(segs, render.Seg(colors.Surface+"· "+detail+colors.Reset, 4))
	}

	return render.Join(segs, " ", width)
}

// renderRows は入力全体を 1 行 1 JSON の出力に変える。id の無いタスクと本体が
// 空になったタスクは出さず、既定の描画に任せる。
func renderRows(in input) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	for _, t := range in.Tasks {
		if t.ID == "" {
			continue
		}
		content := renderTask(t, int(in.Columns))
		if content == "" {
			continue
		}
		if err := enc.Encode(row{ID: t.ID, Content: content}); err != nil {
			return ""
		}
	}
	return b.String()
}

func main() {
	var in input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		// 何も出さなければ Claude Code は既定の行を描く。
		return
	}
	os.Stdout.WriteString(renderRows(in))
}
