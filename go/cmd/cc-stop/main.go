package main

import (
	"encoding/json"
	"os"
	"runtime"

	"github.com/tak848/dotfiles/go/internal/tts"
)

func main() {
	var input map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	gitCtx := tts.GitContext()
	tts.Speak("Claudeセッション終了！"+gitCtx, tts.Neural2Voices)
}
