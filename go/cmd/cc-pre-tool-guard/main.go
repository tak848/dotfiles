package main

import (
	"encoding/json"
	"os"

	"github.com/tak848/dotfiles/go/internal/claudehooks"
)

func main() {
	var input claudehooks.HookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return
	}

	cfg, err := claudehooks.LoadConfig()
	if err != nil {
		return
	}

	decision := claudehooks.EvaluatePreTool(cfg, input)
	if decision == nil {
		return
	}

	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": decision.Reason,
			"additionalContext":        decision.AdditionalContext,
		},
	})
}
