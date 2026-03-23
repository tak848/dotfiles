package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tak848/dotfiles/go/internal/colors"
)

const barWidth = 25

var blocks = [...]rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

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
	RateLimits *struct {
		FiveHour *struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       int64   `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay *struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       int64   `json:"resets_at"`
		} `json:"seven_day"`
	} `json:"rate_limits"`
	SessionID string `json:"session_id"`
	Version   string `json:"version"`
}

func contextColor(pct int) string {
	switch {
	case pct >= 80:
		return colors.Red
	case pct >= 50:
		return colors.Yellow
	default:
		return colors.Green
	}
}

func buildBar(pct int) string {
	pct = max(0, min(pct, 100))
	ctxClr := contextColor(pct)

	filledChars := pct * barWidth / 100
	remainder := (pct * barWidth % 100) * 8 / 100
	emptyChars := barWidth - filledChars
	if remainder > 0 {
		emptyChars--
	}

	var b strings.Builder
	b.WriteString(ctxClr)
	b.WriteString(strings.Repeat("█", filledChars))
	if remainder > 0 {
		b.WriteRune(blocks[remainder])
	}
	b.WriteString(colors.Reset)
	b.WriteString(strings.Repeat(" ", emptyChars))
	b.WriteString(colors.Surface)
	b.WriteRune('▏')
	b.WriteString(colors.Reset)
	return b.String()
}

func formatResetTime(epoch int64, now time.Time, weekly bool) string {
	if epoch <= 0 {
		return ""
	}
	t := time.Unix(epoch, 0)
	if !t.After(now) {
		return ""
	}
	if weekly {
		weekdays := [...]string{"日", "月", "火", "水", "木", "金", "土"}
		return fmt.Sprintf("(~%d/%d%s)", int(t.Month()), t.Day(), weekdays[t.Weekday()])
	}
	return fmt.Sprintf("(~%d:%02d)", t.Hour(), t.Minute())
}

func main() {
	var d Data
	if err := json.NewDecoder(os.Stdin).Decode(&d); err != nil {
		return
	}

	now := time.Now()

	pct := max(0, min(int(d.ContextWindow.UsedPercentage), 100))
	bar := buildBar(pct)
	ctxClr := contextColor(pct)

	usedK := (d.ContextWindow.CurrentUsage.InputTokens + d.ContextWindow.CurrentUsage.CacheReadInputTokens) / 1000
	maxK := d.ContextWindow.ContextWindowSize / 1000

	secs := d.Cost.TotalDurationMs / 1000
	mins := secs / 60
	secs = secs % 60

	sid := d.SessionID
	if len(sid) > 8 {
		sid = sid[:8]
	}

	sep := colors.Surface + " | " + colors.Reset

	// Line 1: model + progress bar + percentage + tokens
	fmt.Printf("%s[%s]%s %s %s%d%%%s (%dk/%dk)\n",
		colors.Teal, d.Model.DisplayName, colors.Reset,
		bar,
		ctxClr, pct, colors.Reset,
		usedK, maxK)

	// Rate limit info (Pro/Max subscribers only)
	var ratePart string
	if d.RateLimits != nil {
		var parts []string
		if d.RateLimits.FiveHour != nil {
			clr := contextColor(int(d.RateLimits.FiveHour.UsedPercentage))
			rt := formatResetTime(d.RateLimits.FiveHour.ResetsAt, now, false)
			parts = append(parts, fmt.Sprintf("5h:%s%.0f%%%s%s", clr, d.RateLimits.FiveHour.UsedPercentage, colors.Reset, rt))
		}
		if d.RateLimits.SevenDay != nil {
			clr := contextColor(int(d.RateLimits.SevenDay.UsedPercentage))
			rt := formatResetTime(d.RateLimits.SevenDay.ResetsAt, now, true)
			parts = append(parts, fmt.Sprintf("7d:%s%.0f%%%s%s", clr, d.RateLimits.SevenDay.UsedPercentage, colors.Reset, rt))
		}
		if len(parts) > 0 {
			ratePart = sep + strings.Join(parts, " ")
		}
	}

	// Line 2: cost | duration | lines | rate limit | session | version
	fmt.Printf("%s$%.2f%s%s%d:%02d%s%s+%d%s %s-%d%s%s%s%s%s%s%s%sv%s%s\n",
		colors.Green, d.Cost.TotalCostUSD, colors.Reset,
		sep, mins, secs,
		sep, colors.Green, d.Cost.TotalLinesAdded, colors.Reset,
		colors.Red, d.Cost.TotalLinesRemoved, colors.Reset,
		ratePart,
		sep, colors.Blue, sid, colors.Reset,
		sep, colors.Lavender, d.Version, colors.Reset)
}
