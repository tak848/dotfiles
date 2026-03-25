package claudehooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const anthropicMessagesURL = "https://api.anthropic.com/v1/messages"

type PermissionDecision struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

func DecidePermission(ctx context.Context, cfg Config, input HookInput) (PermissionDecision, bool, error) {
	if deny := EvaluatePreTool(cfg, input); deny != nil {
		return PermissionDecision{
			Behavior: "deny",
			Message:  deny.Reason,
		}, true, nil
	}

	if decision, ok := localPermissionDecision(input); ok {
		return decision, true, nil
	}

	if strings.ToLower(cfg.Provider.Name) != "anthropic" {
		return PermissionDecision{}, false, nil
	}

	apiKey := os.Getenv("CC_AUTOMODE_ANTHROPIC_API_KEY")
	if apiKey == "" {
		return PermissionDecision{}, false, nil
	}

	decision, err := callAnthropic(ctx, cfg, input, apiKey)
	if err != nil {
		return PermissionDecision{}, false, err
	}
	if decision.Behavior == "" {
		return PermissionDecision{}, false, nil
	}
	return decision, true, nil
}

func localPermissionDecision(input HookInput) (PermissionDecision, bool) {
	if input.ToolName != "Bash" {
		return PermissionDecision{}, false
	}

	command := stringValue(input.ToolInput, "command")
	if command == "" {
		return PermissionDecision{}, false
	}

	lowered := strings.ToLower(command)
	if strings.Contains(lowered, "git push --force") || strings.Contains(lowered, "git reset --hard") {
		return PermissionDecision{
			Behavior: "deny",
			Message:  "破壊的な Git 操作は自動許可しません。",
		}, true
	}

	readOnlyPrefixes := []string{
		"ls ",
		"pwd",
		"cat ",
		"head ",
		"tail ",
		"wc ",
		"file ",
		"date ",
		"which ",
		"rg ",
		"grep ",
		"git status",
		"git diff",
		"git log",
		"git show",
		"gh pr view",
		"gh pr diff",
		"gh pr checks",
		"gh run view",
	}
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(command, prefix) {
			return PermissionDecision{Behavior: "allow"}, true
		}
	}

	return PermissionDecision{}, false
}

func callAnthropic(parent context.Context, cfg Config, input HookInput, apiKey string) (PermissionDecision, error) {
	timeout := time.Duration(cfg.Provider.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	systemPrompt := strings.TrimSpace(fmt.Sprintf(
		"あなたは Claude Code の PermissionRequest hook 用ガードです。\n"+
			"返答は JSON のみで、`behavior` は `allow` `deny` `fallthrough` のいずれかです。\n"+
			"`deny` のときだけ `message` を 1 文で含めてください。\n"+
			"曖昧なら `fallthrough` を返してください。\n\n"+
			"Allow rules:\n- %s\n\nSoft deny rules:\n- %s\n\nEnvironment:\n- %s\n",
		strings.Join(cfg.Allow, "\n- "),
		strings.Join(cfg.SoftDeny, "\n- "),
		strings.Join(cfg.Environment, "\n- "),
	))

	userPayload, err := json.Marshal(map[string]any{
		"tool_name":              input.ToolName,
		"tool_input":             input.ToolInput,
		"cwd":                    input.Cwd,
		"permission_mode":        input.PermissionMode,
		"permission_suggestions": input.PermissionSuggestions,
	})
	if err != nil {
		return PermissionDecision{}, err
	}

	requestBody := map[string]any{
		"model":      cfg.Provider.Model,
		"max_tokens": 128,
		"system":     systemPrompt,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "text",
						"text": string(userPayload),
					},
				},
			},
		},
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return PermissionDecision{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesURL, bytes.NewReader(body))
	if err != nil {
		return PermissionDecision{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return PermissionDecision{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return PermissionDecision{}, errors.New(resp.Status)
	}

	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return PermissionDecision{}, err
	}

	var text string
	for _, block := range payload.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return PermissionDecision{}, nil
	}

	var decision PermissionDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		start := strings.IndexByte(text, '{')
		end := strings.LastIndexByte(text, '}')
		if start >= 0 && end > start {
			if err := json.Unmarshal([]byte(text[start:end+1]), &decision); err != nil {
				return PermissionDecision{}, err
			}
		} else {
			return PermissionDecision{}, err
		}
	}

	switch decision.Behavior {
	case "allow", "deny":
		return decision, nil
	case "fallthrough", "":
		return PermissionDecision{}, nil
	default:
		return PermissionDecision{}, nil
	}
}
