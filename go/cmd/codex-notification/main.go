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

	cmd := exec.Command("say", input.LastAssistantMessage)
	_ = cmd.Run()
}
