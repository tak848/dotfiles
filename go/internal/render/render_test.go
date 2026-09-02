package render

import (
	"strings"
	"testing"
	"time"

	"github.com/tak848/dotfiles/go/internal/colors"
)

// envOf はテスト用の環境変数参照を作る。t.Setenv は t.Parallel() と併用すると
// panic するため、環境は必ず引数で渡す。
func envOf(kv map[string]string) Env {
	return func(k string) string { return kv[k] }
}

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   string
		want string
	}{
		"plain":        {in: "hello", want: "hello"},
		"newline":      {in: "a\nb", want: "ab"},
		"carriage":     {in: "a\rb", want: "ab"},
		"tab":          {in: "a\tb", want: "ab"},
		"escape":       {in: "a\033[31mb", want: "a[31mb"},
		"bel":          {in: "a\x07b", want: "ab"},
		"del":          {in: "a\x7fb", want: "ab"},
		"osc_terminal": {in: "\033]8;;evil\x07x", want: "]8;;evilx"},
		"japanese":     {in: "検索中", want: "検索中"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Sanitize(tt.in); got != tt.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWidth(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   string
		want int
	}{
		"ascii":         {in: "abc", want: 3},
		"empty":         {in: "", want: 0},
		"csi_ignored":   {in: colors.Red + "abc" + colors.Reset, want: 3},
		"osc_ignored":   {in: "\033]8;;https://example.com\a#12\033]8;;\a", want: 3},
		"japanese":      {in: "検索中", want: 6},
		"mixed":         {in: "a検b", want: 4},
		"bar_is_narrow": {in: "███▏", want: 4},
		"worktree_mark": {in: "⎇ x", want: 3},
		// 結合文字は幅 0。NFD で分解された形を明示的に確かめる。
		"combining_nfd": {in: "e\u0301", want: 1},
		"combining":     {in: "é", want: 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Width(tt.in); got != tt.want {
				t.Errorf("Width(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   string
		max  int
		want string
	}{
		"fits":     {in: "abc", max: 5, want: "abc"},
		"exact":    {in: "abc", max: 3, want: "abc"},
		"cut":      {in: "abcdef", max: 3, want: "abc" + colors.Reset},
		"zero":     {in: "abc", max: 0, want: ""},
		"negative": {in: "abc", max: -1, want: ""},
		"keeps_ansi": {
			in:   colors.Red + "abcdef" + colors.Reset,
			max:  3,
			want: colors.Red + "abc" + colors.Reset,
		},
		// 全角は 2 セルなので、幅 3 では 1 文字しか入らない。
		"wide": {in: "検索中", max: 3, want: "検" + colors.Reset},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := Truncate(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
			if Width(got) > max(tt.max, 0) {
				t.Errorf("Truncate(%q, %d) width = %d, exceeds max", tt.in, tt.max, Width(got))
			}
		})
	}
}

func TestTruncateNeverSplitsEscape(t *testing.T) {
	t.Parallel()

	in := colors.Red + "abcdef" + colors.Reset
	for maxw := 1; maxw <= 8; maxw++ {
		got := Truncate(in, maxw)
		if strings.Count(got, "\033") > 0 && !strings.HasSuffix(got, "m") {
			t.Errorf("Truncate(%q, %d) = %q ends mid-escape", in, maxw, got)
		}
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()

	segs := []Segment{
		Seg("anchor", 0),
		Seg("", 1),
		Seg("mid", 2),
		Seg("tail", 3),
	}

	tests := map[string]struct {
		width int
		want  string
	}{
		"unlimited":   {width: 0, want: "anchor|mid|tail"},
		"fits":        {width: 100, want: "anchor|mid|tail"},
		"drop_tail":   {width: 12, want: "anchor|mid"},
		"drop_both":   {width: 8, want: "anchor"},
		"below_floor": {width: 3, want: "anc" + colors.Reset},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Join(segs, "|", tt.width); got != tt.want {
				t.Errorf("Join(width=%d) = %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}

func TestJoinDropsLaterOnTie(t *testing.T) {
	t.Parallel()

	segs := []Segment{Seg("keep", 0), Seg("aaa", 1), Seg("bbb", 1)}
	got := Join(segs, " ", 9)
	if got != "keep aaa" {
		t.Errorf("Join() = %q, want %q (later segment of equal Drop should go first)", got, "keep aaa")
	}
}

func TestHyperlinks(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env  map[string]string
		want bool
	}{
		"default":         {env: map[string]string{"TERM": "xterm-ghostty"}, want: true},
		"explicit_off":    {env: map[string]string{"TERM": "xterm-ghostty", "CC_STATUSLINE_HYPERLINKS": "0"}, want: false},
		"explicit_on":     {env: map[string]string{"TERM": "dumb", "CC_STATUSLINE_HYPERLINKS": "1"}, want: true},
		"explicit_off_no": {env: map[string]string{"TERM": "xterm", "CC_STATUSLINE_HYPERLINKS": "no"}, want: false},
		"force":           {env: map[string]string{"TERM": "dumb", "FORCE_HYPERLINK": "1"}, want: true},
		"no_color":        {env: map[string]string{"TERM": "xterm", "NO_COLOR": "1"}, want: false},
		"dumb":            {env: map[string]string{"TERM": "dumb"}, want: false},
		"no_term":         {env: map[string]string{}, want: false},
		// 明示指定は NO_COLOR より優先する。
		"explicit_beats_no_color": {env: map[string]string{"TERM": "xterm", "NO_COLOR": "1", "CC_STATUSLINE_HYPERLINKS": "on"}, want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Hyperlinks(envOf(tt.env)); got != tt.want {
				t.Errorf("Hyperlinks(%v) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestColumns(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env  map[string]string
		want int
	}{
		"set":      {env: map[string]string{"COLUMNS": "80"}, want: 80},
		"unset":    {env: map[string]string{}, want: 120},
		"empty":    {env: map[string]string{"COLUMNS": ""}, want: 120},
		"garbage":  {env: map[string]string{"COLUMNS": "wide"}, want: 120},
		"zero":     {env: map[string]string{"COLUMNS": "0"}, want: 120},
		"negative": {env: map[string]string{"COLUMNS": "-5"}, want: 120},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Columns(envOf(tt.env), 120); got != tt.want {
				t.Errorf("Columns(%v) = %d, want %d", tt.env, got, tt.want)
			}
		})
	}
}

func TestLink(t *testing.T) {
	t.Parallel()

	const text = "#1234"
	linked := "\033]8;;https://example.com/pull/1234\a" + text + "\033]8;;\a"

	tests := map[string]struct {
		url     string
		enabled bool
		want    string
	}{
		"https":         {url: "https://example.com/pull/1234", enabled: true, want: linked},
		"disabled":      {url: "https://example.com/pull/1234", enabled: false, want: text},
		"empty":         {url: "", enabled: true, want: text},
		"javascript":    {url: "javascript:alert(1)", enabled: true, want: text},
		"file":          {url: "file:///etc/passwd", enabled: true, want: text},
		"no_host":       {url: "https://", enabled: true, want: text},
		"control_chars": {url: "https://example.com/\x07evil", enabled: true, want: text},
		"newline":       {url: "https://example.com/\nx", enabled: true, want: text},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Link(tt.url, text, tt.enabled); got != tt.want {
				t.Errorf("Link(%q, enabled=%v) = %q, want %q", tt.url, tt.enabled, got, tt.want)
			}
		})
	}
}

func TestUsageColor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pct  int
		want string
	}{
		"green_low":       {pct: 0, want: colors.Green},
		"green_boundary":  {pct: 49, want: colors.Green},
		"yellow_boundary": {pct: 50, want: colors.Yellow},
		"yellow_high":     {pct: 79, want: colors.Yellow},
		"red_boundary":    {pct: 80, want: colors.Red},
		"red_max":         {pct: 100, want: colors.Red},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := UsageColor(tt.pct); got != tt.want {
				t.Errorf("UsageColor(%d) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestBar(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pct      int
		width    int
		wantFull int
	}{
		"negative": {pct: -10, width: 25, wantFull: 0},
		"empty":    {pct: 0, width: 25, wantFull: 0},
		"full":     {pct: 100, width: 25, wantFull: 25},
		"over_100": {pct: 150, width: 25, wantFull: 25},
		"half":     {pct: 50, width: 25, wantFull: 12},
		"one_cell": {pct: 4, width: 25, wantFull: 1},
		"narrow":   {pct: 100, width: 5, wantFull: 5},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bar := Bar(tt.pct, tt.width)
			full := strings.Count(bar, "█")
			if full != tt.wantFull {
				t.Errorf("Bar(%d, %d): got %d full blocks, want %d", tt.pct, tt.width, full, tt.wantFull)
			}
			if !strings.ContainsRune(bar, '▏') {
				t.Errorf("Bar(%d, %d) should contain end marker ▏", tt.pct, tt.width)
			}
		})
	}
}

func TestBarZeroWidth(t *testing.T) {
	t.Parallel()

	if got := Bar(50, 0); got != "" {
		t.Errorf("Bar(50, 0) = %q, want empty", got)
	}
}

func TestBarPartialBlock(t *testing.T) {
	t.Parallel()

	bar := Bar(42, 25)
	hasPartial := false
	for _, r := range bar {
		for _, b := range blocks[1:8] {
			if r == b {
				hasPartial = true
			}
		}
	}
	if !hasPartial {
		t.Error("Bar(42, 25) should contain a partial block character")
	}
}

func TestResetTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.Local)

	tests := map[string]struct {
		epoch  float64
		weekly bool
		want   string
	}{
		"zero":     {epoch: 0, weekly: false, want: ""},
		"negative": {epoch: -1, weekly: false, want: ""},
		"past":     {epoch: float64(now.Add(-time.Hour).Unix()), weekly: false, want: ""},
		// resets_at / expires_at 到達時に再実行されるので、この境界が実際に踏まれる。
		"exactly_now":         {epoch: float64(now.Unix()), weekly: false, want: ""},
		"one_second_ahead":    {epoch: float64(now.Add(time.Second).Unix()), weekly: false, want: "(~10:00)"},
		"future_5h":           {epoch: float64(time.Date(2026, 3, 23, 15, 30, 0, 0, time.Local).Unix()), weekly: false, want: "(~15:30)"},
		"future_7d":           {epoch: float64(time.Date(2026, 3, 25, 0, 0, 0, 0, time.Local).Unix()), weekly: true, want: "(~3/25(水) 0:00)"},
		"future_7d_with_time": {epoch: float64(time.Date(2026, 3, 25, 9, 5, 0, 0, time.Local).Unix()), weekly: true, want: "(~3/25(水) 9:05)"},
		"year_crossing":       {epoch: float64(time.Date(2027, 1, 2, 3, 4, 0, 0, time.Local).Unix()), weekly: true, want: "(~1/2(土) 3:04)"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := ResetTime(tt.epoch, now, tt.weekly); got != tt.want {
				t.Errorf("ResetTime(%v, weekly=%v) = %q, want %q", tt.epoch, tt.weekly, got, tt.want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		ms   float64
		want string
	}{
		"zero":     {ms: 0, want: "0:00"},
		"negative": {ms: -1000, want: "0:00"},
		"seconds":  {ms: 5000, want: "0:05"},
		"minute":   {ms: 65000, want: "1:05"},
		"long":     {ms: 754000, want: "12:34"},
		"hours":    {ms: 3_600_000, want: "60:00"},
		// 上流が小数を出しても壊れない。
		"fractional": {ms: 5500.5, want: "0:05"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Duration(tt.ms); got != tt.want {
				t.Errorf("Duration(%v) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}

func TestShortModel(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		id   string
		want string
	}{
		"opus":        {id: "claude-opus-5", want: "opus"},
		"sonnet_date": {id: "claude-sonnet-4-5-20250929", want: "sonnet"},
		"haiku":       {id: "claude-haiku-4-5-20251001", want: "haiku"},
		"legacy":      {id: "claude-3-5-haiku-20241022", want: "haiku"},
		"unknown":     {id: "gpt-5.6-sol", want: "gpt"},
		"bare":        {id: "custom", want: "custom"},
		"empty":       {id: "", want: ""},
		"digits_only": {id: "claude-1-2", want: "claude-1-2"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := ShortModel(tt.id); got != tt.want {
				t.Errorf("ShortModel(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
