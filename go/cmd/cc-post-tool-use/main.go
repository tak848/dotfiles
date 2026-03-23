package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Input struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
	ToolResponse struct {
		Success bool `json:"success"`
	} `json:"tool_response"`
}

func main() {
	var input Input
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	editTools := map[string]struct{}{
		"Write":     {},
		"Edit":      {},
		"MultiEdit": {},
	}
	if _, ok := editTools[input.ToolName]; !ok {
		return
	}

	if !input.ToolResponse.Success || input.ToolInput.FilePath == "" {
		return
	}

	content, err := os.ReadFile(input.ToolInput.FilePath)
	if err != nil {
		return
	}

	if len(content) > 0 && content[len(content)-1] != '\n' {
		f, err := os.OpenFile(input.ToolInput.FilePath, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to add newline to %s: %v\n", input.ToolInput.FilePath, err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString("\n"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to add newline to %s: %v\n", input.ToolInput.FilePath, err)
			return
		}
		fmt.Fprintf(os.Stderr, "Added newline to end of file: %s\n", input.ToolInput.FilePath)
	}
}
