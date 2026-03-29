package main

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/tak848/dotfiles/go/internal/tts"
)

type Input struct {
	Type                 string `json:"type"`
	LastAssistantMessage string `json:"last-assistant-message"`
}

func main() {
	if tts.HandleBackground() {
		return
	}

	// Codex passes notification payload as the last CLI argument
	if len(os.Args) < 2 {
		return
	}

	var input Input
	if err := json.Unmarshal([]byte(os.Args[len(os.Args)-1]), &input); err != nil {
		return
	}

	if input.Type != "agent-turn-complete" {
		return
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	tts.SpeakInBackground(notificationMessage(input), tts.DefaultVoices)
}

func notificationMessage(input Input) string {
	const base = "Codex の応答が終わりました。"

	summary := summarizeAssistantMessage(input.LastAssistantMessage, 80)
	if summary == "" {
		return base
	}
	return base + " " + summary
}

func summarizeAssistantMessage(message string, limit int) string {
	if limit <= 0 {
		return ""
	}

	line := strings.TrimSpace(strings.Split(message, "\n")[0])
	if line == "" {
		return ""
	}

	line = strings.Join(strings.Fields(line), " ")
	if line == "" {
		return ""
	}

	if utf8.RuneCountInString(line) <= limit {
		return line
	}

	runes := []rune(line)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
