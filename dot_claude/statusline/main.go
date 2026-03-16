package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Catppuccin Mocha colors
const (
	teal     = "\033[38;2;148;226;213m"
	green    = "\033[38;2;166;227;161m"
	yellow   = "\033[38;2;249;226;175m"
	red      = "\033[38;2;243;139;168m"
	blue     = "\033[38;2;137;180;250m"
	lavender = "\033[38;2;180;190;254m"
	surface  = "\033[38;2;88;91;112m"
	reset    = "\033[0m"
)

type Data struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage    float64 `json:"used_percentage"`
		ContextWindowSize int     `json:"context_window_size"`
		CurrentUsage      struct {
			InputTokens          int `json:"input_tokens"`
			CacheReadInputTokens int `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	Cost struct {
		TotalCostUSD      float64 `json:"total_cost_usd"`
		TotalDurationMs   int     `json:"total_duration_ms"`
		TotalLinesAdded   int     `json:"total_lines_added"`
		TotalLinesRemoved int     `json:"total_lines_removed"`
	} `json:"cost"`
	SessionID string `json:"session_id"`
	Version   string `json:"version"`
}

func contextColor(pct int) string {
	switch {
	case pct >= 80:
		return red
	case pct >= 50:
		return yellow
	default:
		return green
	}
}

func main() {
	var d Data
	if err := json.NewDecoder(os.Stdin).Decode(&d); err != nil {
		return
	}

	pct := min(int(d.ContextWindow.UsedPercentage), 100)
	filled := min(pct/5, 20)
	ctxClr := contextColor(pct)
	bar := ctxClr + strings.Repeat("▓", filled) + strings.Repeat("░", 20-filled) + reset

	usedK := (d.ContextWindow.CurrentUsage.InputTokens + d.ContextWindow.CurrentUsage.CacheReadInputTokens) / 1000
	maxK := d.ContextWindow.ContextWindowSize / 1000

	secs := d.Cost.TotalDurationMs / 1000
	mins := secs / 60
	secs = secs % 60

	sid := d.SessionID
	if len(sid) > 8 {
		sid = sid[:8]
	}

	sep := surface + " | " + reset

	// Line 1: model + progress bar + percentage + tokens
	fmt.Printf("%s[%s]%s %s %s%d%%%s (%dk/%dk)\n",
		teal, d.Model.DisplayName, reset,
		bar,
		ctxClr, pct, reset,
		usedK, maxK)

	// Line 2: cost | duration | lines | session | version
	fmt.Printf("%s$%.2f%s%s%d:%02d%s%s+%d%s %s-%d%s%s%s%s%s%s%sv%s%s\n",
		green, d.Cost.TotalCostUSD, reset,
		sep, mins, secs,
		sep, green, d.Cost.TotalLinesAdded, reset,
		red, d.Cost.TotalLinesRemoved, reset,
		sep, blue, sid, reset,
		sep, lavender, d.Version, reset)
}
