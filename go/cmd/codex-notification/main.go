package main

import (
	"encoding/json"
	"os"
	"runtime"

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

	if input.LastAssistantMessage == "" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		return
	}

	tts.SpeakInBackground(input.LastAssistantMessage, tts.DefaultVoices)
}
