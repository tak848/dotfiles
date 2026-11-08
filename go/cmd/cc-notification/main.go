package main

import (
	"encoding/json/v2"
	"os"
	"runtime"

	"github.com/tak848/dotfiles/go/internal/tts"
)

type Input struct {
	Message string `json:"message"`
	Title   string `json:"title"`
}

func main() {
	// 読み上げは AGENT_TTS で明示的に有効にしたときだけ。stdin も読まずに終わる。
	if !tts.Enabled(os.Getenv) {
		return
	}

	var input Input
	if err := json.UnmarshalRead(os.Stdin, &input, json.MatchCaseInsensitiveNames(true)); err != nil {
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
	tts.Speak(message+" "+gitCtx, tts.DefaultVoices)
}
