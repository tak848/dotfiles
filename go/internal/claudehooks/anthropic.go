package claudehooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/invopop/jsonschema"
)

type PermissionDecision struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

type PermissionLLMOutput struct {
	Behavior    string `json:"behavior" jsonschema_description:"One of allow, deny, fallthrough."`
	DenyMessage string `json:"deny_message,omitempty" jsonschema_description:"Required when behavior is deny. A short Japanese explanation shown to Claude Code."`
	Reasoning   string `json:"reasoning" jsonschema_description:"Short explanation of why this decision was chosen."`
}

type PermissionPromptInput struct {
	ToolName              string            `json:"tool_name"`
	ToolInput             HookToolInput     `json:"tool_input"`
	ToolInputRaw          json.RawMessage   `json:"tool_input_raw,omitempty"`
	PermissionMode        string            `json:"permission_mode"`
	PermissionSuggestions []json.RawMessage `json:"permission_suggestions,omitempty"`
	Context               PermissionContext `json:"context"`
}

func DecidePermission(ctx context.Context, cfg Config, input HookInput) (PermissionDecision, bool, error) {
	if strings.ToLower(cfg.Provider.Name) != "anthropic" {
		slog.Info("provider not anthropic, skipping", "provider", cfg.Provider.Name)
		return PermissionDecision{}, false, nil
	}

	apiKey := strings.TrimSpace(os.Getenv("CC_AUTOMODE_ANTHROPIC_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	}
	if apiKey == "" {
		slog.Warn("no API key found (CC_AUTOMODE_ANTHROPIC_API_KEY / ANTHROPIC_API_KEY)")
		return PermissionDecision{}, false, nil
	}

	slog.Info("calling anthropic",
		"model", cfg.Provider.Model,
		"timeout_ms", cfg.Provider.TimeoutMS,
		"tool", input.ToolName,
	)

	output, err := callAnthropic(ctx, cfg, input, apiKey)
	if err != nil {
		slog.Error("anthropic API call failed", "error", err, "tool", input.ToolName)
		return PermissionDecision{}, false, err
	}

	slog.Info("LLM decision",
		"behavior", output.Behavior,
		"reasoning", output.Reasoning,
		"deny_message", output.DenyMessage,
		"tool", input.ToolName,
	)

	switch output.Behavior {
	case "allow":
		return PermissionDecision{Behavior: "allow"}, true, nil
	case "deny":
		message := strings.TrimSpace(output.DenyMessage)
		if message == "" {
			message = "危険な可能性が高いため、自動許可しません。"
		}
		return PermissionDecision{Behavior: "deny", Message: message}, true, nil
	case "fallthrough", "":
		return PermissionDecision{}, false, nil
	default:
		slog.Warn("unexpected LLM behavior", "behavior", output.Behavior)
		return PermissionDecision{}, false, nil
	}
}

func callAnthropic(parent context.Context, cfg Config, input HookInput, apiKey string) (PermissionLLMOutput, error) {
	timeout := time.Duration(cfg.Provider.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	perRetryTimeout := timeout / 3
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(perRetryTimeout),
		option.WithMaxRetries(5),
	)

	systemPrompt := permissionSystemPrompt(cfg)
	promptInput := PermissionPromptInput{
		ToolName:              input.ToolName,
		ToolInput:             input.ToolInput,
		ToolInputRaw:          input.ToolInputRaw,
		PermissionMode:        input.PermissionMode,
		PermissionSuggestions: input.PermissionSuggestions,
		Context:               BuildPermissionContext(input),
	}
	userMessage := mustJSON(promptInput)

	slog.Info("anthropic request",
		"system_prompt", systemPrompt,
		"user_message", mustJSON(redactPromptInput(promptInput)),
	)

	message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(cfg.Provider.Model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{
				Text: systemPrompt,
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{
				Schema: permissionOutputSchema(),
			},
		},
		Temperature: anthropic.Float(0),
	})
	if err != nil {
		return PermissionLLMOutput{}, err
	}

	text := extractMessageText(message)
	slog.Info("anthropic response", "raw", text)
	if text == "" {
		return PermissionLLMOutput{}, nil
	}

	var output PermissionLLMOutput
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		return PermissionLLMOutput{}, fmt.Errorf("JSON parse error: %w (raw: %.200s)", err, text)
	}
	if output.Behavior == "deny" && strings.TrimSpace(output.DenyMessage) == "" {
		output.DenyMessage = "危険な可能性が高いため、自動許可しません。"
	}

	return output, nil
}

func permissionSystemPrompt(cfg Config) string {
	var b strings.Builder
	b.WriteString("You are a PermissionRequest hook classifier for Claude Code.\n")
	b.WriteString("Return one of: allow, deny, fallthrough.\n")
	b.WriteString("Decide quickly. Do not deliberate or reconsider. Keep reasoning under 2 sentences.\n")
	b.WriteString("Deny guidance rules are mandatory. If a rule matches, deny immediately.\n")
	b.WriteString("Use allow only when the operation clearly matches allow guidance.\n")
	b.WriteString("Use fallthrough for anything uncertain or not clearly matching allow guidance.\n")
	b.WriteString("When deny, provide a concise Japanese deny_message.\n\n")

	if len(cfg.Allow) > 0 {
		b.WriteString("Allow guidance:\n- ")
		b.WriteString(strings.Join(cfg.Allow, "\n- "))
		b.WriteString("\n\n")
	}
	if len(cfg.Deny) > 0 {
		b.WriteString("Deny guidance (mandatory):\n- ")
		b.WriteString(strings.Join(cfg.Deny, "\n- "))
		b.WriteString("\n\n")
	}
	if len(cfg.Environment) > 0 {
		b.WriteString("Environment:\n- ")
		b.WriteString(strings.Join(cfg.Environment, "\n- "))
	}

	return strings.TrimSpace(b.String())
}

func permissionOutputSchema() map[string]any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect(PermissionLLMOutput{})
	data, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{"type": "object"}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{"type": "object"}
	}
	return out
}

func extractMessageText(message *anthropic.Message) string {
	if message == nil {
		return ""
	}
	var text strings.Builder
	for _, block := range message.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			text.WriteString(variant.Text)
		}
	}
	return strings.TrimSpace(text.String())
}

func redactPromptInput(p PermissionPromptInput) PermissionPromptInput {
	const mask = "[REDACTED]"
	r := p
	if r.ToolInput.Content != "" {
		r.ToolInput.Content = mask
	}
	if len(r.ToolInput.ContentUpdates) > 0 {
		r.ToolInput.ContentUpdates = nil
	}
	r.ToolInputRaw = nil
	r.PermissionSuggestions = nil
	return r
}

func mustJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
