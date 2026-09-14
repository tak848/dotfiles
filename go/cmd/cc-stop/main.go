package main

import (
	"encoding/json/v2"
	"os"
	"runtime"

	"github.com/tak848/dotfiles/go/internal/tts"
)

func main() {
	var input struct{}
	if err := json.UnmarshalRead(os.Stdin, &input); err != nil {
		return
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	gitCtx := tts.GitContext()
	tts.Speak("Claudeセッション終了！"+gitCtx, tts.Neural2Voices)
}
