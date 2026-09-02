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

// placeholder は値がまだ届いていない箇所を埋める。セグメントごと消すと値が
// 来た瞬間に行が横に伸びて表示が飛ぶので、枠は残して中身だけ伏せる。
const placeholder = "--"

// reviewMark は PR のレビュー状態を記号と色に対応づける。
var reviewMark = map[string][2]string{
	"approved":          {"✓", colors.Green},
	"changes_requested": {"✗", colors.Red},
	"pending":           {"…", colors.Yellow},
	"draft":             {"◌", colors.Surface},
}

// placeholderTime は時刻がまだ分からないときの伏せ字。実際の表記と桁を
// 揃えてあるので、値が入っても幅が動かない。
func placeholderTime(weekly bool) string {
	if weekly {
		return colors.Surface + "(~--/--(-) --:--)" + colors.Reset
	}
	return colors.Surface + "(~--:--)" + colors.Reset
}

// resetTime は絶対時刻を返す。未設定や到達済みのときは伏せ字で枠を残す。
// resets_at / expires_at の到達時にも statusline は再実行されるので、
// その瞬間だけ表記が消えるのを防ぐ。
func resetTime(epoch float64, now time.Time, weekly bool) string {
	if s := render.ResetTime(epoch, now, weekly); s != "" {
		return s
	}
	return placeholderTime(weekly)
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
		if lv := render.Sanitize(d.Effort.Level); lv != "" {
			badge += "·" + lv
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

	// 使用率がまだ来ていなくても、空のバーと伏せた数値で枠を出す。セッション
	// 初期や /compact 直後にセグメントごと消えると、次の応答で急に行が伸びる。
	pct := -1
	if d.ContextWindow.UsedPercentage != nil {
		pct = max(0, min(int(*d.ContextWindow.UsedPercentage), 100))
	}
	pctText := colors.Surface + placeholder + "%" + colors.Reset
	if pct >= 0 {
		pctText = render.UsageColor(pct) + strconv.Itoa(pct) + "%" + colors.Reset
	}
	segs = append(segs,
		render.Seg(render.Bar(max(pct, 0), barWidth(width)), 1),
		render.Seg(pctText, 0),
	)

	used := placeholder
	if u := d.ContextWindow.CurrentUsage; u != nil {
		used = strconv.Itoa(int(u.InputTokens+u.CacheCreationInputTokens+u.CacheReadInputTokens) / 1000)
	}
	size := placeholder
	if d.ContextWindow.ContextWindowSize > 0 {
		size = strconv.Itoa(int(d.ContextWindow.ContextWindowSize) / 1000)
	}
	segs = append(segs, render.Seg(fmt.Sprintf("(%sk/%sk)", used, size), 1))

	return render.Join(segs, " ", width)
}

// cacheSegment は prompt cache の要約を組み立てる。統計が届くのは main
// conversation の初回 API 応答後なので、それまでは枠だけ出しておく。
// キャッシュを観測できない環境（プロキシ越しなど）でも同じ形にする。
func cacheSegment(d *Data, now time.Time) string {
	pc := d.PromptCache
	if pc == nil || !pc.CachingObserved {
		return colors.Surface +
			strings.Join([]string{"cache", placeholder, placeholder, placeholder + "%", "miss0"}, " ") +
			colors.Reset
	}

	ttl := placeholder
	if pc.TTL != "" {
		ttl = render.Sanitize(pc.TTL)
	}

	state := colors.Red + "cold" + colors.Reset
	if pc.Warm {
		// 期限到達時は Claude Code が statusline を再実行するので、そのとき
		// 時刻が消えると同時に cold へ落ちる。
		expires := placeholderTime(false)
		if pc.ExpiresAt != nil {
			expires = resetTime(*pc.ExpiresAt, now, false)
		}
		state = colors.Green + "warm" + colors.Reset + expires
	}

	ratio := colors.Surface + placeholder + "%" + colors.Reset
	if pc.HitRatio != nil {
		ratio = strconv.Itoa(int(*pc.HitRatio*100)) + "%"
	}

	miss := colors.Surface + "miss0" + colors.Reset
	if pc.Misses > 0 {
		miss = colors.Yellow + "miss" + strconv.Itoa(int(pc.Misses)) + colors.Reset
	}

	return strings.Join([]string{"cache", ttl, state, ratio, miss}, " ")
}

// rateSegment はレート制限を組み立てる。rate_limits も初回 API 応答までは
// 来ないので、その間は伏せた値で枠を出す。spend_limit は Claude apps gateway
// 配下でしか生成されないため、あるときだけ足す。
func rateSegment(d *Data, now time.Time) string {
	var five, seven, spend *Window
	if d.RateLimits != nil {
		five, seven, spend = d.RateLimits.FiveHour, d.RateLimits.SevenDay, d.RateLimits.SpendLimit
	}

	format := func(label string, w *Window, weekly bool) string {
		if w == nil {
			return label + ":" + colors.Surface + placeholder + "%" + colors.Reset + placeholderTime(weekly)
		}
		return fmt.Sprintf("%s:%s%.0f%%%s%s", label,
			render.UsageColor(int(w.UsedPercentage)), w.UsedPercentage, colors.Reset,
			resetTime(w.ResetsAt, now, weekly))
	}

	parts := []string{format("5h", five, false), format("7d", seven, true)}
	if spend != nil {
		parts = append(parts, format("$", spend, true))
	}
	return strings.Join(parts, " ")
}

// renderCostLine は 2 行目を組み立てる。
func renderCostLine(d *Data, now time.Time, width int) string {
	// 経過時間と API 時間を並べると「2 つの時間」で読みにくいので、API 側は
	// 占有率にする。まだ API 応答が無いうちも括弧ごと残して幅を保つ。
	api := colors.Surface + "(api " + placeholder + "%)" + colors.Reset
	if d.Cost.TotalDurationMs > 0 && d.Cost.TotalAPIDurationMs > 0 {
		api = fmt.Sprintf("(api %.0f%%)", d.Cost.TotalAPIDurationMs/d.Cost.TotalDurationMs*100)
	}
	elapsed := render.Duration(d.Cost.TotalDurationMs) + " " + api

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
// リンクは番号そのものに掛ける。PR は作業中に現れたり消えたりするので、
// 無いときも枠を残す。
func prSegment(d *Data, links bool) string {
	if d.PR == nil || d.PR.Number <= 0 {
		return colors.Surface + "#" + placeholder + colors.Reset
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
