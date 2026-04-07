package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type input struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type writeInput struct {
	Content string `json:"content"`
}

type editInput struct {
	NewString string `json:"new_string"`
}

type multiEditInput struct {
	Edits []editInput `json:"edits"`
}

type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func main() {
	var in input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		return
	}

	var found []string

	switch in.ToolName {
	case "Write":
		var ti writeInput
		if err := json.Unmarshal(in.ToolInput, &ti); err != nil {
			return
		}
		if strings.ContainsRune(ti.Content, '\uFFFD') {
			found = append(found, "content")
		}
	case "Edit":
		var ti editInput
		if err := json.Unmarshal(in.ToolInput, &ti); err != nil {
			return
		}
		if strings.ContainsRune(ti.NewString, '\uFFFD') {
			found = append(found, "new_string")
		}
	case "MultiEdit":
		var ti multiEditInput
		if err := json.Unmarshal(in.ToolInput, &ti); err != nil {
			return
		}
		for i, e := range ti.Edits {
			if strings.ContainsRune(e.NewString, '\uFFFD') {
				found = append(found, fmt.Sprintf("edits[%d].new_string", i))
			}
		}
	default:
		return
	}

	if len(found) == 0 {
		return
	}

	for _, f := range found {
		fmt.Fprintf(os.Stderr, "mojibake detected in field: %s\n", f)
	}

	out := hookOutput{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "deny",
			PermissionDecisionReason: fmt.Sprintf(
				"U+FFFD (文字化け) を検出しました。影響箇所を書き直してください。Fields: %s",
				strings.Join(found, ", "),
			),
		},
	}
	json.NewEncoder(os.Stdout).Encode(out)
}
