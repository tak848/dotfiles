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

	_ = json.NewEncoder(os.Stdout).Encode(preToolResponse{
		HookSpecificOutput: preToolOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: decision.Reason,
			AdditionalContext:        decision.AdditionalContext,
		},
	})
}

type preToolResponse struct {
	HookSpecificOutput preToolOutput `json:"hookSpecificOutput"`
}

type preToolOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}
