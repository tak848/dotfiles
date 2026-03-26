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

	cfg, err := claudehooks.LoadConfig(input.Cwd)
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
		},
		SystemMessage: decision.SystemMessage,
	})
}

type preToolResponse struct {
	HookSpecificOutput preToolOutput `json:"hookSpecificOutput"`
	SystemMessage      string        `json:"systemMessage,omitempty"`
}

type preToolOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}
