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

	command := input.ToolInput.Command
	if command == "" {
		return PermissionDecision{}, false
	}

	if containsShellMeta(command) {
		return PermissionDecision{}, false
	}

	tokens := shellSplit(command)
	if len(tokens) == 0 {
		return PermissionDecision{}, false
	}

	lowered := strings.ToLower(command)
	if isDangerousGitCommand(tokens, lowered) {
		return PermissionDecision{
			Behavior: "deny",
			Message:  "破壊的な Git 操作は自動許可しません。",
		}, true
	}

	if isReadOnlyCommand(tokens) {
		return PermissionDecision{Behavior: "allow"}, true
	}

	return PermissionDecision{}, false
}

func containsShellMeta(command string) bool {
	metaTokens := []string{"&&", "||", ";", "|", "`", "$(", ">", "<"}
	for _, token := range metaTokens {
		if strings.Contains(command, token) {
			return true
		}
	}
	return false
}

func isDangerousGitCommand(tokens []string, lowered string) bool {
	if len(tokens) < 2 || tokens[0] != "git" {
		return false
	}

	if tokens[1] == "push" {
		for _, token := range tokens[2:] {
			if token == "-f" || token == "--force" || token == "--force-with-lease" {
				return true
			}
		}
	}

	if tokens[1] == "reset" {
		for _, token := range tokens[2:] {
			if token == "--hard" {
				return true
			}
		}
	}

	return strings.Contains(lowered, "git push --force") || strings.Contains(lowered, "git reset --hard")
}

func isReadOnlyCommand(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}

	switch tokens[0] {
	case "pwd", "date":
		return len(tokens) == 1
	case "ls", "cat", "head", "tail", "wc", "file", "which", "rg", "grep":
		return true
	case "git":
		if len(tokens) < 2 {
			return false
		}
		switch tokens[1] {
		case "status", "diff", "log", "show":
			return true
		}
	case "gh":
		if len(tokens) < 3 {
			return false
		}
		if tokens[1] == "pr" && (tokens[2] == "view" || tokens[2] == "diff" || tokens[2] == "checks") {
			return true
		}
		if tokens[1] == "run" && tokens[2] == "view" {
			return true
		}
	}

	return false
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

	userPayload, err := json.Marshal(struct {
		ToolName              string            `json:"tool_name"`
		ToolInput             HookToolInput     `json:"tool_input"`
		Cwd                   string            `json:"cwd"`
		PermissionMode        string            `json:"permission_mode"`
		PermissionSuggestions []json.RawMessage `json:"permission_suggestions"`
	}{
		ToolName:              input.ToolName,
		ToolInput:             input.ToolInput,
		Cwd:                   input.Cwd,
		PermissionMode:        input.PermissionMode,
		PermissionSuggestions: input.PermissionSuggestions,
	})
	if err != nil {
		return PermissionDecision{}, err
	}

	requestBody := struct {
		Model     string                `json:"model"`
		MaxTokens int                   `json:"max_tokens"`
		System    string                `json:"system"`
		Messages  []anthropicMessageReq `json:"messages"`
	}{
		Model:     cfg.Provider.Model,
		MaxTokens: 128,
		System:    systemPrompt,
		Messages: []anthropicMessageReq{
			{
				Role: "user",
				Content: []anthropicTextBlockReq{
					{
						Type: "text",
						Text: string(userPayload),
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

type anthropicMessageReq struct {
	Role    string                  `json:"role"`
	Content []anthropicTextBlockReq `json:"content"`
}

type anthropicTextBlockReq struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
