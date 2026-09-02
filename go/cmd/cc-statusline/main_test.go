package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tak848/dotfiles/go/internal/render"
)

func ptr[T any](v T) *T { return &v }

// stripANSI は表示内容だけを取り出す。セパレータの重複判定などに使う。
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) {
				c := s[i]
				i++
				if c == 0x07 || (c >= 0x40 && c <= 0x7e && i > 1 && s[i-2] != 0x1b) {
					break
				}
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// checkLine は全ケースに共通する不変条件を確かめる。行が増える・崩れるバグは
// 個別の期待文字列より、この 4 つで捕まえたほうが確実に効く。
func checkLine(t *testing.T, line string, width int) {
	t.Helper()

	if strings.ContainsAny(line, "\n\r") {
		t.Errorf("line contains a newline, which would add a phantom row: %q", line)
	}
	if w := render.Width(line); w > width {
		t.Errorf("line width = %d, exceeds %d: %q", w, width, line)
	}

	plain := stripANSI(line)
	if strings.Contains(plain, "|  |") || strings.Contains(plain, " |  ") {
		t.Errorf("line has an empty segment between separators: %q", plain)
	}
	if strings.HasPrefix(strings.TrimSpace(plain), "|") || strings.HasSuffix(strings.TrimSpace(plain), "|") {
		t.Errorf("line starts or ends with a separator: %q", plain)
	}
}

// fullData は全フィールドが揃った入力。各テストはここから必要な部分を削る。
func fullData() *Data {
	d := &Data{}
	d.Model.DisplayName = "Opus"
	d.ContextWindow.UsedPercentage = ptr(25.0)
	d.ContextWindow.ContextWindowSize = 1_000_000
	d.ContextWindow.CurrentUsage = &struct {
		InputTokens              float64 `json:"input_tokens"`
		CacheCreationInputTokens float64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     float64 `json:"cache_read_input_tokens"`
	}{InputTokens: 50_000, CacheReadInputTokens: 200_000}
	d.Cost.TotalCostUSD = 1.23
	d.Cost.TotalDurationMs = 754_000
	d.Cost.TotalAPIDurationMs = 251_000
	d.Cost.TotalLinesAdded = 156
	d.Cost.TotalLinesRemoved = 23
	d.Workspace.GitWorktree = "feat-x"
	d.Workspace.Repo = &struct {
		Host  string `json:"host"`
		Owner string `json:"owner"`
		Name  string `json:"name"`
	}{Host: "github.com", Owner: "tak848", Name: "dotfiles"}
	d.OutputStyle.Name = "default"
	d.Effort = &struct {
		Level string `json:"level"`
	}{Level: "high"}
	d.Thinking.Enabled = true
	d.PromptCache = &struct {
		Warm            bool     `json:"warm"`
		CachingObserved bool     `json:"caching_observed"`
		TTL             string   `json:"ttl"`
		ExpiresAt       *float64 `json:"expires_at"`
		Misses          float64  `json:"misses"`
		HitRatio        *float64 `json:"hit_ratio"`
	}{Warm: true, CachingObserved: true, TTL: "1h", Misses: 2, HitRatio: ptr(0.91)}
	d.RateLimits = &struct {
		FiveHour   *Window `json:"five_hour"`
		SevenDay   *Window `json:"seven_day"`
		SpendLimit *Window `json:"spend_limit"`
	}{
		FiveHour: &Window{UsedPercentage: 23.5},
		SevenDay: &Window{UsedPercentage: 41.2},
	}
	d.PR = &struct {
		Number      float64 `json:"number"`
		URL         string  `json:"url"`
		ReviewState string  `json:"review_state"`
		Kind        string  `json:"kind"`
	}{Number: 1234, URL: "https://github.com/tak848/dotfiles/pull/1234", ReviewState: "approved"}
	d.SessionID = "abc12345-6789-0000"
	d.SessionName = "my-session"
	d.Version = "2.1.257"
	return d
}

func TestRenderModelLine(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate      func(*Data)
		wantContain []string
		wantAbsent  []string
	}{
		"full": {
			mutate:      func(*Data) {},
			wantContain: []string{"[Opus·H]", "⎇ feat-x", "25%", "(250k/1000k)"},
		},
		"no_effort": {
			mutate:      func(d *Data) { d.Effort = nil },
			wantContain: []string{"[Opus]"},
		},
		"unknown_effort": {
			mutate:      func(d *Data) { d.Effort.Level = "turbo" },
			wantContain: []string{"[Opus·turbo]"},
		},
		"fast_mode": {
			mutate:      func(d *Data) { d.FastMode = true },
			wantContain: []string{"[Opus·H·fast]"},
		},
		"thinking_off": {
			mutate:      func(d *Data) { d.Thinking.Enabled = false },
			wantContain: []string{"nothink"},
		},
		"output_style": {
			mutate:      func(d *Data) { d.OutputStyle.Name = "explanatory" },
			wantContain: []string{"‹explanatory›"},
		},
		"agent": {
			mutate: func(d *Data) {
				d.Agent = &struct {
					Name string `json:"name"`
				}{Name: "reviewer"}
			},
			wantContain: []string{"@reviewer"},
		},
		// 使用率が未確定のうちは 0% の緑バーではなく、値が無いことを示す。
		"percentage_null": {
			mutate:      func(d *Data) { d.ContextWindow.UsedPercentage = nil },
			wantContain: []string{"--%"},
			wantAbsent:  []string{"0%"},
		},
		// current_usage が null のときに 0k と出すと嘘になるので出さない。
		"usage_null": {
			mutate:     func(d *Data) { d.ContextWindow.CurrentUsage = nil },
			wantAbsent: []string{"0k"},
		},
		"no_worktree": {
			mutate:     func(d *Data) { d.Workspace.GitWorktree = "" },
			wantAbsent: []string{"⎇"},
		},
		"empty_model": {
			mutate:      func(d *Data) { d.Model.DisplayName = "" },
			wantContain: []string{"[?·H]"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := fullData()
			tt.mutate(d)
			got := renderModelLine(d, 120)
			checkLine(t, got, 120)
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("renderModelLine() = %q, want it to contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(stripANSI(got), absent) {
					t.Errorf("renderModelLine() = %q, want it not to contain %q", got, absent)
				}
			}
		})
	}
}

func TestRenderCostLine(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.Local)
	soon := float64(now.Add(time.Hour).Unix())

	tests := map[string]struct {
		mutate      func(*Data)
		wantContain []string
		wantAbsent  []string
	}{
		"full": {
			mutate:      func(d *Data) { d.PromptCache.ExpiresAt = ptr(soon) },
			wantContain: []string{"$1.23", "12:34", "(api 33%)", "+156", "-23", "cache 1h", "warm(~11:00)", "91%", "miss2", "5h:", "7d:"},
		},
		"no_prompt_cache": {
			mutate:     func(d *Data) { d.PromptCache = nil },
			wantAbsent: []string{"cache"},
		},
		// キャッシュを観測できない環境では統計に意味が無いので出さない。
		"caching_not_observed": {
			mutate:     func(d *Data) { d.PromptCache.CachingObserved = false },
			wantAbsent: []string{"cache"},
		},
		"cold": {
			mutate:      func(d *Data) { d.PromptCache.Warm = false },
			wantContain: []string{"cold"},
			wantAbsent:  []string{"warm"},
		},
		// 期限を過ぎた expires_at は時刻を出さない。warm 表示だけが残る。
		"expired": {
			mutate:      func(d *Data) { d.PromptCache.ExpiresAt = ptr(float64(now.Add(-time.Hour).Unix())) },
			wantContain: []string{"warm"},
			wantAbsent:  []string{"(~"},
		},
		"hit_ratio_null": {
			mutate:      func(d *Data) { d.PromptCache.HitRatio = nil },
			wantContain: []string{"cache 1h"},
			wantAbsent:  []string{"%|"},
		},
		"no_misses": {
			mutate:     func(d *Data) { d.PromptCache.Misses = 0 },
			wantAbsent: []string{"miss"},
		},
		"no_rate_limits": {
			mutate:     func(d *Data) { d.RateLimits = nil },
			wantAbsent: []string{"5h:", "7d:"},
		},
		"only_five_hour": {
			mutate:      func(d *Data) { d.RateLimits.SevenDay = nil },
			wantContain: []string{"5h:"},
			wantAbsent:  []string{"7d:"},
		},
		"spend_limit": {
			mutate:      func(d *Data) { d.RateLimits.SpendLimit = &Window{UsedPercentage: 62.8} },
			wantContain: []string{"$:"},
		},
		"no_api_duration": {
			mutate:     func(d *Data) { d.Cost.TotalAPIDurationMs = 0 },
			wantAbsent: []string{"api"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := fullData()
			tt.mutate(d)
			got := renderCostLine(d, now, 200)
			checkLine(t, got, 200)
			for _, want := range tt.wantContain {
				if !strings.Contains(stripANSI(got), want) {
					t.Errorf("renderCostLine() = %q, want it to contain %q", stripANSI(got), want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(stripANSI(got), absent) {
					t.Errorf("renderCostLine() = %q, want it not to contain %q", stripANSI(got), absent)
				}
			}
		})
	}
}

func TestRenderSessionLine(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate      func(*Data)
		links       bool
		wantContain []string
		wantAbsent  []string
	}{
		"full": {
			mutate:      func(*Data) {},
			wantContain: []string{"tak848/dotfiles", "#1234", "✓", "my-session", "abc12345", "v2.1.257"},
		},
		"no_repo": {
			mutate:     func(d *Data) { d.Workspace.Repo = nil },
			wantAbsent: []string{"tak848/dotfiles"},
		},
		"no_pr": {
			mutate:     func(d *Data) { d.PR = nil },
			wantAbsent: []string{"#1234"},
		},
		"merge_request": {
			mutate:      func(d *Data) { d.PR.Kind = "mr" },
			wantContain: []string{"!1234"},
			wantAbsent:  []string{"#1234"},
		},
		"changes_requested": {
			mutate:      func(d *Data) { d.PR.ReviewState = "changes_requested" },
			wantContain: []string{"✗"},
		},
		"draft": {
			mutate:      func(d *Data) { d.PR.ReviewState = "draft" },
			wantContain: []string{"◌"},
		},
		// review_state だけが単独で欠落することがある。
		"no_review_state": {
			mutate:      func(d *Data) { d.PR.ReviewState = "" },
			wantContain: []string{"#1234"},
		},
		"no_session_name": {
			mutate:      func(d *Data) { d.SessionName = "" },
			wantContain: []string{"abc12345"},
			wantAbsent:  []string{"my-session"},
		},
		"remote": {
			mutate: func(d *Data) {
				d.Remote = &struct {
					SessionID string `json:"session_id"`
				}{SessionID: "x"}
			},
			wantContain: []string{"⇄"},
		},
		// 非 git ディレクトリでは repo も PR も無い。行は最小形になるが空にはならない。
		"minimal": {
			mutate: func(d *Data) {
				d.Workspace.Repo = nil
				d.PR = nil
				d.SessionName = ""
			},
			wantContain: []string{"abc12345", "v2.1.257"},
		},
		"links_enabled": {
			mutate:      func(*Data) {},
			links:       true,
			wantContain: []string{"\033]8;;https://github.com/tak848/dotfiles\a"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := fullData()
			tt.mutate(d)
			got := renderSessionLine(d, 200, tt.links)
			checkLine(t, got, 200)
			hay := got
			if !tt.links {
				hay = stripANSI(got)
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(hay, want) {
					t.Errorf("renderSessionLine() = %q, want it to contain %q", hay, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(stripANSI(got), absent) {
					t.Errorf("renderSessionLine() = %q, want it not to contain %q", stripANSI(got), absent)
				}
			}
		})
	}
}

// TestSanitizesHostileStrings は外部由来の文字列に制御文字が混ざっても行が
// 増えないことを確かめる。session_name はユーザーが自由に付けられる。
func TestSanitizesHostileStrings(t *testing.T) {
	t.Parallel()

	d := fullData()
	d.SessionName = "evil\nsecond line"
	d.Workspace.GitWorktree = "wt\033[31m"
	d.Model.DisplayName = "Opus\x07"

	for _, line := range []string{
		renderModelLine(d, 120),
		renderSessionLine(d, 120, true),
	} {
		checkLine(t, line, 120)
	}
}

// TestDecodeNullsAreNotZero は null が 0 に化けないことを確かめる。
// encoding/json は非ポインタ型に null を入れても no-op で通してしまうので、
// ここが崩れると /compact 直後に「0k・0%」という嘘が表示される。
func TestDecodeNullsAreNotZero(t *testing.T) {
	t.Parallel()

	const input = `{
		"model": {"display_name": "Opus"},
		"context_window": {
			"used_percentage": null,
			"remaining_percentage": null,
			"context_window_size": 200000,
			"current_usage": null
		},
		"prompt_cache": {"caching_observed": true, "warm": true, "ttl": "1h", "expires_at": null, "hit_ratio": null},
		"session_id": "abc12345",
		"version": "2.1.257"
	}`

	var d Data
	if err := json.Unmarshal([]byte(input), &d); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if d.ContextWindow.UsedPercentage != nil {
		t.Errorf("UsedPercentage = %v, want nil", *d.ContextWindow.UsedPercentage)
	}
	if d.ContextWindow.CurrentUsage != nil {
		t.Error("CurrentUsage should stay nil so that it is distinguishable from zero tokens")
	}
	if d.PromptCache == nil || d.PromptCache.HitRatio != nil || d.PromptCache.ExpiresAt != nil {
		t.Error("prompt_cache null fields should stay nil")
	}
}

// TestDecodeAcceptsFractionalNumbers は epoch やトークン数が小数で来ても
// decode 全体が落ちないことを確かめる。int64 で受けているとここで失敗し、
// statusline が丸ごと消える。
func TestDecodeAcceptsFractionalNumbers(t *testing.T) {
	t.Parallel()

	const input = `{
		"cost": {"total_cost_usd": 1.5, "total_duration_ms": 754000.5, "total_lines_added": 156.0},
		"context_window": {"context_window_size": 200000.0, "current_usage": {"input_tokens": 8500.5}},
		"rate_limits": {"five_hour": {"used_percentage": 23.5, "resets_at": 1756800000.0}},
		"prompt_cache": {"caching_observed": true, "expires_at": 1756800000.5, "misses": 2.0},
		"pr": {"number": 1234.0, "url": "https://example.com/pull/1234"}
	}`

	var d Data
	if err := json.Unmarshal([]byte(input), &d); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if d.RateLimits == nil || d.RateLimits.FiveHour == nil || d.RateLimits.FiveHour.ResetsAt == 0 {
		t.Error("fractional resets_at should decode")
	}
	if d.PR == nil || int(d.PR.Number) != 1234 {
		t.Error("fractional pr.number should decode")
	}
}

func TestBarWidthShrinksWithTerminal(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		width int
		want  int
	}{
		"wide":   {width: 200, want: 25},
		"medium": {width: 100, want: 15},
		"narrow": {width: 60, want: 8},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := barWidth(tt.width); got != tt.want {
				t.Errorf("barWidth(%d) = %d, want %d", tt.width, got, tt.want)
			}
		})
	}
}

// TestNarrowTerminalKeepsAnchors は端末が狭くても各行の要となる情報が
// 残ることを確かめる。行が空になると statusline の高さが変わってしまう。
func TestNarrowTerminalKeepsAnchors(t *testing.T) {
	t.Parallel()

	d := fullData()
	for _, width := range []int{40, 60, 80} {
		model := renderModelLine(d, width)
		cost := renderCostLine(d, time.Now(), width)
		session := renderSessionLine(d, width, false)

		for _, line := range []string{model, cost, session} {
			checkLine(t, line, width)
			if strings.TrimSpace(stripANSI(line)) == "" {
				t.Errorf("width %d produced an empty line", width)
			}
		}
		if !strings.Contains(stripANSI(model), "Opus") {
			t.Errorf("width %d dropped the model badge: %q", width, stripANSI(model))
		}
		if !strings.Contains(stripANSI(cost), "$") {
			t.Errorf("width %d dropped the cost: %q", width, stripANSI(cost))
		}
		if !strings.Contains(stripANSI(session), "v2.1.257") {
			t.Errorf("width %d dropped the version: %q", width, stripANSI(session))
		}
	}
}
