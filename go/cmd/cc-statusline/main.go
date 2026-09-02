// cc-statusline は Claude Code の statusLine コマンド。stdin の JSON を読んで
// 3 行を stdout に書く。
//
//	1 行目: モデルとその状態、worktree、コンテキスト使用率
//	2 行目: コスト、経過時間、増減行数、prompt cache、レート制限
//	3 行目: repo、PR、セッション、バージョン
//
// 各行には必ず残るセグメント（Drop が 0 のもの）を置いてある。行が空になると
// fullscreen renderer で statusline の高さが変わり、入力欄が上下に跳ねるため。
//
// 欠落しうるフィールドはすべてポインタで受ける。encoding/json は非ポインタ型に
// null を入れても no-op でゼロ値のまま通すので、ポインタにしないと
// 「current_usage が null」と「本当に 0 トークン」を区別できず、/compact 直後に
// もっともらしい嘘を表示することになる。数値は int ではなく float64 で受ける。
// 上流が epoch を小数付きで出した瞬間に decode 全体が落ちて statusline が
// 消えるのを避けるため。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tak848/dotfiles/go/internal/colors"
	"github.com/tak848/dotfiles/go/internal/render"
)

// Window はレート制限の 1 ウィンドウ。
type Window struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       float64 `json:"resets_at"`
}

// Data は statusLine コマンドに渡る JSON。フィールドの意味と欠落条件は
// https://code.claude.com/docs/en/statusline.md を参照。
type Data struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage    *float64 `json:"used_percentage"`
		ContextWindowSize float64  `json:"context_window_size"`
		CurrentUsage      *struct {
			InputTokens              float64 `json:"input_tokens"`
			CacheCreationInputTokens float64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     float64 `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	Cost struct {
		TotalCostUSD       float64 `json:"total_cost_usd"`
		TotalDurationMs    float64 `json:"total_duration_ms"`
		TotalAPIDurationMs float64 `json:"total_api_duration_ms"`
		TotalLinesAdded    float64 `json:"total_lines_added"`
		TotalLinesRemoved  float64 `json:"total_lines_removed"`
	} `json:"cost"`
	Workspace struct {
		GitWorktree string `json:"git_worktree"`
		Repo        *struct {
			Host  string `json:"host"`
			Owner string `json:"owner"`
			Name  string `json:"name"`
		} `json:"repo"`
	} `json:"workspace"`
	OutputStyle struct {
		Name string `json:"name"`
	} `json:"output_style"`
	// Effort は effort パラメータ非対応のモデルでは欠落する。
	Effort *struct {
		Level string `json:"level"`
	} `json:"effort"`
	Thinking struct {
		Enabled bool `json:"enabled"`
	} `json:"thinking"`
	FastMode bool `json:"fast_mode"`
	// PromptCache は v2.1.251 以降、main conversation の初回 API 応答後に現れる。
	PromptCache *struct {
		Warm            bool     `json:"warm"`
		CachingObserved bool     `json:"caching_observed"`
		TTL             string   `json:"ttl"`
		ExpiresAt       *float64 `json:"expires_at"`
		Misses          float64  `json:"misses"`
		HitRatio        *float64 `json:"hit_ratio"`
	} `json:"prompt_cache"`
	// RateLimits は Pro/Max 加入者と、spend limit を課す gateway 配下でだけ来る。
	// SpendLimit は gateway 経由のときのみ。
	RateLimits *struct {
		FiveHour   *Window `json:"five_hour"`
		SevenDay   *Window `json:"seven_day"`
		SpendLimit *Window `json:"spend_limit"`
	} `json:"rate_limits"`
	Agent *struct {
		Name string `json:"name"`
	} `json:"agent"`
	PR *struct {
		Number      float64 `json:"number"`
		URL         string  `json:"url"`
		ReviewState string  `json:"review_state"`
		Kind        string  `json:"kind"`
	} `json:"pr"`
	// Remote は公式ドキュメントに記載が無いが、remote session では実際に来る。
	Remote *struct {
		SessionID string `json:"session_id"`
	} `json:"remote"`
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	Version     string `json:"version"`
}

const (
	// defaultColumns は COLUMNS が読めないときの想定幅。
	defaultColumns = 120
	// rightMargin は verbose モードのトークンカウンタと通知が右端に重なる分の余白。
	rightMargin = 12
)

// effortShort は effort レベルの短縮表記。
var effortShort = map[string]string{
	"low":    "L",
	"medium": "M",
	"high":   "H",
	"xhigh":  "X",
	"max":    "MAX",
}

// reviewMark は PR のレビュー状態を記号と色に対応づける。
var reviewMark = map[string][2]string{
	"approved":          {"✓", colors.Green},
	"changes_requested": {"✗", colors.Red},
	"pending":           {"…", colors.Yellow},
	"draft":             {"◌", colors.Surface},
}

func barWidth(width int) int {
	switch {
	case width >= 120:
		return 25
	case width >= 90:
		return 15
	default:
		return 8
	}
}

// modelBadge はモデル名に effort・fast mode・thinking の状態を畳み込む。
// thinking は既定で有効なので、無効のときだけ出す。
func modelBadge(d *Data) string {
	badge := render.Sanitize(d.Model.DisplayName)
	if badge == "" {
		badge = "?"
	}
	if d.Effort != nil {
		if s, ok := effortShort[d.Effort.Level]; ok {
			badge += "·" + s
		} else if d.Effort.Level != "" {
			badge += "·" + render.Sanitize(d.Effort.Level)
		}
	}
	if d.FastMode {
		badge += "·fast"
	}
	out := colors.Teal + "[" + badge + "]" + colors.Reset
	if !d.Thinking.Enabled {
		out = colors.Teal + "[" + badge + colors.Reset + colors.Red + "·nothink" + colors.Reset + colors.Teal + "]" + colors.Reset
	}
	return out
}

// renderModelLine は 1 行目を組み立てる。
func renderModelLine(d *Data, width int) string {
	segs := []render.Segment{render.Seg(modelBadge(d), 0)}

	if s := render.Sanitize(d.OutputStyle.Name); s != "" && s != "default" {
		segs = append(segs, render.Seg(colors.Lavender+"‹"+s+"›"+colors.Reset, 4))
	}
	if d.Agent != nil {
		if n := render.Sanitize(d.Agent.Name); n != "" {
			segs = append(segs, render.Seg(colors.Blue+"@"+n+colors.Reset, 3))
		}
	}
	if wt := render.Sanitize(d.Workspace.GitWorktree); wt != "" {
		segs = append(segs, render.Seg(colors.Peach+"⎇ "+wt+colors.Reset, 2))
	}

	if d.ContextWindow.UsedPercentage == nil {
		// セッション初期はまだ使用率が分からない。バーを出すと 0% の緑バーに
		// 見えてしまうので、値が無いことをそのまま示す。
		segs = append(segs, render.Seg(colors.Surface+"--%"+colors.Reset, 0))
	} else {
		pct := max(0, min(int(*d.ContextWindow.UsedPercentage), 100))
		segs = append(segs,
			render.Seg(render.Bar(pct, barWidth(width)), 1),
			render.Seg(render.UsageColor(pct)+strconv.Itoa(pct)+"%"+colors.Reset, 0),
		)
	}

	if u := d.ContextWindow.CurrentUsage; u != nil {
		used := int(u.InputTokens+u.CacheCreationInputTokens+u.CacheReadInputTokens) / 1000
		size := int(d.ContextWindow.ContextWindowSize) / 1000
		segs = append(segs, render.Seg(fmt.Sprintf("(%dk/%dk)", used, size), 1))
	}

	return render.Join(segs, " ", width)
}

// cacheSegment は prompt cache の要約を組み立てる。キャッシュが観測できていない
// 環境（プロキシ越しなど）では何も出さない。
func cacheSegment(d *Data, now time.Time) string {
	pc := d.PromptCache
	if pc == nil || !pc.CachingObserved {
		return ""
	}

	parts := []string{"cache"}
	if pc.TTL != "" {
		parts = append(parts, render.Sanitize(pc.TTL))
	}
	if pc.Warm {
		state := colors.Green + "warm" + colors.Reset
		if pc.ExpiresAt != nil {
			// 期限到達時は Claude Code が statusline を再実行するので、
			// そのタイミングで時刻が消えると同時に cold へ落ちる。
			if at := render.ResetTime(*pc.ExpiresAt, now, false); at != "" {
				state += at
			}
		}
		parts = append(parts, state)
	} else {
		parts = append(parts, colors.Red+"cold"+colors.Reset)
	}
	if pc.HitRatio != nil {
		parts = append(parts, strconv.Itoa(int(*pc.HitRatio*100))+"%")
	}
	if pc.Misses > 0 {
		parts = append(parts, colors.Yellow+"miss"+strconv.Itoa(int(pc.Misses))+colors.Reset)
	}
	return strings.Join(parts, " ")
}

func rateSegment(d *Data, now time.Time) string {
	if d.RateLimits == nil {
		return ""
	}

	var parts []string
	add := func(label string, w *Window, weekly bool) {
		if w == nil {
			return
		}
		clr := render.UsageColor(int(w.UsedPercentage))
		parts = append(parts, fmt.Sprintf("%s:%s%.0f%%%s%s",
			label, clr, w.UsedPercentage, colors.Reset, render.ResetTime(w.ResetsAt, now, weekly)))
	}
	add("5h", d.RateLimits.FiveHour, false)
	add("7d", d.RateLimits.SevenDay, true)
	add("$", d.RateLimits.SpendLimit, true)

	return strings.Join(parts, " ")
}

// renderCostLine は 2 行目を組み立てる。
func renderCostLine(d *Data, now time.Time, width int) string {
	elapsed := render.Duration(d.Cost.TotalDurationMs)
	if d.Cost.TotalDurationMs > 0 && d.Cost.TotalAPIDurationMs > 0 {
		// 経過時間と API 時間を並べると「2 つの時間」で読みにくいので、
		// API 側は占有率にする。
		elapsed += fmt.Sprintf(" (api %.0f%%)", d.Cost.TotalAPIDurationMs/d.Cost.TotalDurationMs*100)
	}

	segs := []render.Segment{
		render.Seg(fmt.Sprintf("%s$%.2f%s", colors.Green, d.Cost.TotalCostUSD, colors.Reset), 0),
		render.Seg(elapsed, 1),
		render.Seg(fmt.Sprintf("%s+%d%s %s-%d%s",
			colors.Green, int(d.Cost.TotalLinesAdded), colors.Reset,
			colors.Red, int(d.Cost.TotalLinesRemoved), colors.Reset), 2),
		render.Seg(cacheSegment(d, now), 3),
		render.Seg(rateSegment(d, now), 1),
	}
	return render.Join(segs, colors.Surface+" | "+colors.Reset, width)
}

// prSegment は PR 番号とレビュー状態を組み立てる。番号だけでも意味が通るよう、
// リンクは番号そのものに掛ける。
func prSegment(d *Data, links bool) string {
	if d.PR == nil || d.PR.Number <= 0 {
		return ""
	}

	prefix := "#"
	if d.PR.Kind == "mr" {
		prefix = "!"
	}
	text := colors.Blue + prefix + strconv.Itoa(int(d.PR.Number)) + colors.Reset
	out := render.Link(d.PR.URL, text, links)

	if m, ok := reviewMark[d.PR.ReviewState]; ok {
		out += " " + m[1] + m[0] + colors.Reset
	}
	return out
}

// renderSessionLine は 3 行目を組み立てる。OSC 8 のリンクは行の左端に寄せる。
// Claude Code 側が右から切り詰めたときにシーケンスの途中で切れると、以降の
// 端末出力すべてがリンク扱いになるため。
func renderSessionLine(d *Data, width int, links bool) string {
	var segs []render.Segment

	if r := d.Workspace.Repo; r != nil && r.Owner != "" && r.Name != "" {
		name := render.Sanitize(r.Owner + "/" + r.Name)
		url := ""
		if r.Host != "" {
			url = "https://" + render.Sanitize(r.Host) + "/" + name
		}
		segs = append(segs, render.Seg(render.Link(url, colors.Teal+name+colors.Reset, links), 2))
	}
	if s := prSegment(d, links); s != "" {
		segs = append(segs, render.Seg(s, 2))
	}
	if n := render.Sanitize(d.SessionName); n != "" {
		segs = append(segs, render.Seg(colors.Lavender+n+colors.Reset, 3))
	}
	if d.Remote != nil && d.Remote.SessionID != "" {
		segs = append(segs, render.Seg(colors.Peach+"⇄"+colors.Reset, 4))
	}

	sid := render.Sanitize(d.SessionID)
	if len(sid) > 8 {
		sid = sid[:8]
	}
	segs = append(segs,
		render.Seg(colors.Blue+sid+colors.Reset, 1),
		render.Seg(colors.Lavender+"v"+render.Sanitize(d.Version)+colors.Reset, 0),
	)

	return render.Join(segs, colors.Surface+" | "+colors.Reset, width)
}

func main() {
	var d Data
	if err := json.NewDecoder(os.Stdin).Decode(&d); err != nil {
		// 無出力にすると statusline が黙って消え、壊れたことに気づけない。
		fmt.Printf("%scc-statusline: 入力を読めませんでした (%v)%s\n", colors.Red, err, colors.Reset)
		return
	}

	env := render.Env(os.Getenv)
	width := max(render.Columns(env, defaultColumns)-rightMargin, 40)
	links := render.Hyperlinks(env)
	now := time.Now()

	var b strings.Builder
	b.WriteString(renderModelLine(&d, width))
	b.WriteByte('\n')
	b.WriteString(renderCostLine(&d, now, width))
	b.WriteByte('\n')
	b.WriteString(renderSessionLine(&d, width, links))
	b.WriteByte('\n')

	// 実行中に次の更新が来るとプロセスは kill されるので、部分的に書き出された
	// 状態を作らないよう 1 回で流す。
	os.Stdout.WriteString(b.String())
}
