package main

import (
	"context"
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

	decision, ok, err := claudehooks.DecidePermission(context.Background(), cfg, input)
	if err != nil || !ok {
		return
	}

	output := permissionRequestResponse{
		HookSpecificOutput: permissionRequestOutput{
			HookEventName: "PermissionRequest",
			Decision: permissionDecisionOutput{
				Behavior: decision.Behavior,
				Message:  decision.Message,
			},
		},
	}

	_ = json.NewEncoder(os.Stdout).Encode(output)
}

type permissionRequestResponse struct {
	HookSpecificOutput permissionRequestOutput `json:"hookSpecificOutput"`
}

type permissionRequestOutput struct {
	HookEventName string                   `json:"hookEventName"`
	Decision      permissionDecisionOutput `json:"decision"`
}

type permissionDecisionOutput struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}
