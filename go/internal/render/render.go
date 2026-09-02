// Package render は cc-statusline と cc-subagent-statusline が共有する表示
// ユーティリティを提供する。どちらも Claude Code から stdin で JSON を受け、
// 幅の限られた 1 行に収める点が共通しているため、幅計算・切り詰め・
// サニタイズ・セグメント結合をここに集約する。
//
// 外部依存は持たない。go.sum を空のまま保つことで、chezmoi apply 時と CI の
// どちらもネットワーク無しでビルドできる状態を維持する。
package render

import (
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tak848/dotfiles/go/internal/colors"
)

// Sanitize は外部由来の文字列から制御文字を取り除く。session_name・PR の
// タイトル・subagent の description などはユーザー入力や GitHub 由来なので、
// 改行が 1 個混ざるだけで行数が狂い、ESC が混ざれば色や OSC 8 の終端を
// 乗っ取られる。表示に使う前に必ず通す。
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// escapeLen は s の先頭が ANSI エスケープシーケンスならその長さを返す。
// 該当しなければ 0。CSI（色）と OSC（ハイパーリンク）の両方を見る。
func escapeLen(s string) int {
	if len(s) < 2 || s[0] != 0x1b {
		return 0
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
		return len(s)
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	default:
		return 2
	}
}

// runeWidth は 1 文字の表示セル数を返す。結合文字は 0、East Asian の
// Wide/Fullwidth は 2、それ以外は 1 とする。Ambiguous 幅は 1 に倒す
// （既存の進捗バーが █ や ▏ を 1 セル前提で並べているため）。
func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r), unicode.Is(unicode.Cf, r):
		return 0
	case isWide(r):
		return 2
	default:
		return 1
	}
}

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f, // Hangul Jamo
		r >= 0x2e80 && r <= 0x303e,   // CJK 部首補助〜CJK の記号
		r >= 0x3041 && r <= 0x33ff,   // かな〜CJK 互換
		r >= 0x3400 && r <= 0x4dbf,   // CJK 拡張 A
		r >= 0x4e00 && r <= 0x9fff,   // CJK 統合漢字
		r >= 0xa000 && r <= 0xa4cf,   // Yi
		r >= 0xa960 && r <= 0xa97f,   // Hangul Jamo 拡張 A
		r >= 0xac00 && r <= 0xd7a3,   // Hangul 音節
		r >= 0xf900 && r <= 0xfaff,   // CJK 互換漢字
		r >= 0xfe10 && r <= 0xfe19,   // 縦書き形
		r >= 0xfe30 && r <= 0xfe6f,   // CJK 互換形
		r >= 0xff00 && r <= 0xff60,   // 全角 ASCII
		r >= 0xffe0 && r <= 0xffe6,   // 全角記号
		r >= 0x1f300 && r <= 0x1f64f, // 絵文字
		r >= 0x1f900 && r <= 0x1f9ff, // 補助絵文字
		r >= 0x20000 && r <= 0x3fffd: // CJK 拡張 B 以降
		return true
	}
	return false
}

// Width は ANSI エスケープを除いた表示幅を返す。
func Width(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if n := escapeLen(s[i:]); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		w += runeWidth(r)
	}
	return w
}

// Truncate は表示幅が max を超えないよう s を切り詰める。ANSI エスケープは
// 幅 0 として保持するため、シーケンスの途中で切れることはない。切り詰めた
// ときは色が開いたまま残らないよう Reset を付ける。
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if Width(s) <= max {
		return s
	}

	var b strings.Builder
	w := 0
	for i := 0; i < len(s); {
		if n := escapeLen(s[i:]); n > 0 {
			b.WriteString(s[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runeWidth(r)
		if w+rw > max {
			break
		}
		b.WriteRune(r)
		w += rw
		i += size
	}
	b.WriteString(colors.Reset)
	return b.String()
}

// Segment は 1 行を構成する表示単位。Drop は落としやすさで、大きいものから
// 先に捨てる。Drop が 0 のセグメントは幅が足りなくても落とさない
// （行が丸ごと空になると fullscreen renderer で入力欄が上下に跳ねるため、
// 各行に必ず残るアンカーを 1 つ以上置く）。
type Segment struct {
	Text string
	Drop int
}

// Seg は Segment を組み立てる。text が空ならそのセグメントは無かったことに
// なるので、欠落しうるフィールドは空文字を渡せばよい。
func Seg(text string, drop int) Segment {
	return Segment{Text: text, Drop: drop}
}

// Join はセグメントを sep で連結する。width を超える場合は Drop の大きい
// ものから、同じ Drop なら後ろにあるものから落とす。Drop が 0 のものだけに
// なってもまだ溢れるときは切り詰める。width が 0 以下なら幅制限しない。
func Join(segs []Segment, sep string, width int) string {
	keep := make([]bool, len(segs))
	for i, s := range segs {
		keep[i] = s.Text != ""
	}

	for {
		out := joinKept(segs, keep, sep)
		if width <= 0 || Width(out) <= width {
			return out
		}

		victim := -1
		for i, s := range segs {
			if !keep[i] || s.Drop == 0 {
				continue
			}
			if victim < 0 || s.Drop >= segs[victim].Drop {
				victim = i
			}
		}
		if victim < 0 {
			return Truncate(out, width)
		}
		keep[victim] = false
	}
}

func joinKept(segs []Segment, keep []bool, sep string) string {
	parts := make([]string, 0, len(segs))
	for i, s := range segs {
		if keep[i] {
			parts = append(parts, s.Text)
		}
	}
	return strings.Join(parts, sep)
}

// Env は環境変数の参照関数。os.Getenv をそのまま渡せる。テストから環境を
// 差し替えられるよう引数で受けており、これにより t.Setenv を使わずに済む
// （t.Setenv は t.Parallel() と併用すると panic する）。
type Env func(string) string

// Hyperlinks は OSC 8 を出してよいかを環境変数だけで判定する。statusline の
// stdout はパイプなので isatty は使えず、端末の実能力は分からない。明示指定を
// 最優先し、色を出さない環境では無効にする。
func Hyperlinks(env Env) bool {
	switch strings.ToLower(env("CC_STATUSLINE_HYPERLINKS")) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	}
	if env("FORCE_HYPERLINK") != "" {
		return true
	}
	if env("NO_COLOR") != "" {
		return false
	}
	switch env("TERM") {
	case "", "dumb":
		return false
	}
	return true
}

// Columns は端末幅を返す。Claude Code は statusline を実行する前に COLUMNS を
// 設定する（stdout がパイプなので tput cols は使えない）。
func Columns(env Env, fallback int) int {
	n, err := strconv.Atoi(env("COLUMNS"))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// Link は OSC 8 のハイパーリンクを組み立てる。無効時・URL が http(s) でない
// とき・制御文字を含むときは text をそのまま返す。終端子は ST より互換性の
// 高い BEL を使う。
//
// 呼び出し側は text だけで意味が通る文字列を渡すこと。リンクが剥がれても
// 情報が失われないようにするため。
func Link(rawURL, text string, enabled bool) string {
	if !enabled || rawURL == "" || rawURL != Sanitize(rawURL) {
		return text
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return text
	}
	return "\033]8;;" + rawURL + "\a" + text + "\033]8;;\a"
}

// UsageColor は使用率に応じた色を返す。
func UsageColor(pct int) string {
	switch {
	case pct >= 80:
		return colors.Red
	case pct >= 50:
		return colors.Yellow
	default:
		return colors.Green
	}
}

var blocks = [...]rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

// Bar は width セルの進捗バーを返す。末尾には目盛りとして ▏ を置く。
func Bar(pct, width int) string {
	pct = max(0, min(pct, 100))
	if width <= 0 {
		return ""
	}

	filled := pct * width / 100
	remainder := (pct * width % 100) * 8 / 100
	empty := width - filled
	if remainder > 0 {
		empty--
	}

	var b strings.Builder
	b.WriteString(UsageColor(pct))
	b.WriteString(strings.Repeat("█", filled))
	if remainder > 0 {
		b.WriteRune(blocks[remainder])
	}
	b.WriteString(colors.Reset)
	b.WriteString(strings.Repeat(" ", empty))
	b.WriteString(colors.Surface)
	b.WriteRune('▏')
	b.WriteString(colors.Reset)
	return b.String()
}

var weekdays = [...]string{"日", "月", "火", "水", "木", "金", "土"}

// ResetTime は epoch 秒を絶対時刻の括弧表記にする。未設定・過去なら空文字を
// 返す。Claude Code は resets_at / expires_at の到達時に statusline を再実行
// するので、到達直後の一瞬はここが空になる。呼び出し側は時刻が消えるだけで
// なく状態表示（cold など）も切り替わるようにしておくこと。
func ResetTime(epoch float64, now time.Time, weekly bool) string {
	if epoch <= 0 {
		return ""
	}
	t := time.Unix(int64(epoch), 0)
	if !t.After(now) {
		return ""
	}
	if weekly {
		return "(~" + strconv.Itoa(int(t.Month())) + "/" + strconv.Itoa(t.Day()) +
			"(" + weekdays[t.Weekday()] + ") " + clock(t) + ")"
	}
	return "(~" + clock(t) + ")"
}

func clock(t time.Time) string {
	m := strconv.Itoa(t.Minute())
	if len(m) == 1 {
		m = "0" + m
	}
	return strconv.Itoa(t.Hour()) + ":" + m
}

// Duration はミリ秒を m:ss 形式にする。
func Duration(ms float64) string {
	if ms < 0 {
		ms = 0
	}
	secs := int(ms) / 1000
	s := strconv.Itoa(secs % 60)
	if len(s) == 1 {
		s = "0" + s
	}
	return strconv.Itoa(secs/60) + ":" + s
}

// ShortModel はモデル ID を短縮する。claude-sonnet-4-5-20250929 なら sonnet。
// 未知の形式は原型をそのまま返す（新しいモデルで空になるのを避けるため）。
func ShortModel(id string) string {
	id = Sanitize(id)
	if id == "" {
		return ""
	}
	for _, part := range strings.Split(strings.TrimPrefix(id, "claude-"), "-") {
		if part == "" {
			continue
		}
		if _, err := strconv.Atoi(part); err != nil {
			return part
		}
	}
	return id
}
