package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
)

type Input struct {
	Type                 string `json:"type"`
	LastAssistantMessage string `json:"last-assistant-message"`
}

func main() {
	// background child: say command was started by parent
	if os.Getenv("_SAY_BG") == "1" {
		msg := os.Getenv("_SAY_MSG")
		if msg != "" {
			cmd := exec.Command("say", msg)
			_ = cmd.Run()
		}
		return
	}

	// Codex passes notification payload as the last CLI argument, not stdin
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

	if input.LastAssistantMessage == "" || runtime.GOOS != "darwin" {
		return
	}

	// fork self as background process for say
	exe, err := os.Executable()
	if err != nil {
		return // can't fork; skip notification rather than blocking
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "_SAY_BG=1", "_SAY_MSG="+input.LastAssistantMessage)
	_ = cmd.Start()
}
