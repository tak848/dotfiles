package main

import (
	"encoding/json"
	"fmt"
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

	var input Input
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid JSON input: %v\n", err)
		os.Exit(1)
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
