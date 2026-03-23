package main

import (
	"encoding/json"
	"os"
	"runtime"

	"github.com/tak848/dotfiles/go/internal/tts"
)

type Input struct {
	Message string `json:"message"`
	Title   string `json:"title"`
}

func main() {
	if tts.HandleBackground() {
		return
	}

	var input Input
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	message := input.Message
	if message == "" {
		message = input.Title
	}
	if message == "" {
		return
	}

	gitCtx := tts.GitContext()
	tts.SpeakInBackground(message+" "+gitCtx, tts.DefaultVoices)
}
