package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type input struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
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

	editTools := map[string]struct{}{
		"Write":     {},
		"Edit":      {},
		"MultiEdit": {},
	}
	if _, ok := editTools[in.ToolName]; !ok {
		return
	}

	var found []string
	checkValue(in.ToolInput, "", &found)

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

func checkValue(v any, path string, found *[]string) {
	switch val := v.(type) {
	case string:
		if strings.ContainsRune(val, '\uFFFD') {
			*found = append(*found, path)
		}
	case map[string]any:
		for k, child := range val {
			p := k
			if path != "" {
				p = path + "." + k
			}
			checkValue(child, p, found)
		}
	case []any:
		for i, child := range val {
			p := fmt.Sprintf("%s[%d]", path, i)
			checkValue(child, p, found)
		}
	}
}
