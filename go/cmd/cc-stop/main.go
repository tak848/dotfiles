package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/tak848/dotfiles/go/internal/tts"
)

func main() {
	var input map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	gitCtx := tts.GitContext()
	message := "Claudeセッション終了！" + gitCtx
	tts.Speak(message, tts.Neural2Voices)
}
