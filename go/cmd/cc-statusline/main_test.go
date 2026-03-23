package main

import (
	"testing"
	"time"
)

func TestContextColor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pct  int
		want string
	}{
		"green_low":       {pct: 0, want: "\033[38;2;166;227;161m"},
		"green_boundary":  {pct: 49, want: "\033[38;2;166;227;161m"},
		"yellow_boundary": {pct: 50, want: "\033[38;2;249;226;175m"},
		"yellow_high":     {pct: 79, want: "\033[38;2;249;226;175m"},
		"red_boundary":    {pct: 80, want: "\033[38;2;243;139;168m"},
		"red_max":         {pct: 100, want: "\033[38;2;243;139;168m"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := contextColor(tt.pct); got != tt.want {
				t.Errorf("contextColor(%d) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestBuildBar(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pct      int
		wantFull int
	}{
		"negative": {pct: -10, wantFull: 0},
		"empty":    {pct: 0, wantFull: 0},
		"full":     {pct: 100, wantFull: 25},
		"over_100": {pct: 150, wantFull: 25},
		"half":     {pct: 50, wantFull: 12},
		"one_cell": {pct: 4, wantFull: 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bar := buildBar(tt.pct)
			fullCount := 0
			for _, r := range bar {
				if r == '█' {
					fullCount++
				}
			}
			if fullCount != tt.wantFull {
				t.Errorf("buildBar(%d): got %d full blocks, want %d", tt.pct, fullCount, tt.wantFull)
			}
		})
	}
}

func TestBuildBarContainsEndMarker(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pct int
	}{
		"0%":   {pct: 0},
		"25%":  {pct: 25},
		"42%":  {pct: 42},
		"50%":  {pct: 50},
		"75%":  {pct: 75},
		"99%":  {pct: 99},
		"100%": {pct: 100},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bar := buildBar(tt.pct)
			found := false
			for _, r := range bar {
				if r == '▏' {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("buildBar(%d) should contain end marker ▏", tt.pct)
			}
		})
	}
}

func TestBuildBarPartialBlock(t *testing.T) {
	t.Parallel()

	bar := buildBar(42)
	hasPartial := false
	for _, r := range bar {
		for _, b := range blocks[1:8] {
			if r == b {
				hasPartial = true
			}
		}
	}
	if !hasPartial {
		t.Error("buildBar(42) should contain a partial block character")
	}
}

func TestFormatResetTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.Local)

	tests := map[string]struct {
		epoch  int64
		weekly bool
		want   string
	}{
		"zero":      {epoch: 0, weekly: false, want: ""},
		"past":      {epoch: now.Add(-1 * time.Hour).Unix(), weekly: false, want: ""},
		"future_5h": {epoch: time.Date(2026, 3, 23, 15, 30, 0, 0, time.Local).Unix(), weekly: false, want: "(~15:30)"},
		"future_7d": {epoch: time.Date(2026, 3, 25, 0, 0, 0, 0, time.Local).Unix(), weekly: true, want: "(~3/25水)"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := formatResetTime(tt.epoch, now, tt.weekly)
			if got != tt.want {
				t.Errorf("formatResetTime() = %q, want %q", got, tt.want)
			}
		})
	}
}
