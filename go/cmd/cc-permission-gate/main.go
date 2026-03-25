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

	cfg, err := claudehooks.LoadConfig()
	if err != nil {
		return
	}

	decision, ok, err := claudehooks.DecidePermission(context.Background(), cfg, input)
	if err != nil || !ok {
		return
	}

	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName": "PermissionRequest",
			"decision": map[string]any{
				"behavior": decision.Behavior,
			},
		},
	}

	if decision.Behavior == "deny" && decision.Message != "" {
		output["hookSpecificOutput"].(map[string]any)["decision"].(map[string]any)["message"] = decision.Message
	}

	_ = json.NewEncoder(os.Stdout).Encode(output)
}
