package main

import (
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestRunJSONInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, input, field string
	}{
		{"write", `{"tool_name":"Write","tool_input":{"content":"aBADb"}}`, "content"},
		{"edit", `{"tool_name":"Edit","tool_input":{"new_string":"BAD"}}`, "new_string"},
		{"multiedit", `{"tool_name":"MultiEdit","tool_input":{"edits":[{"new_string":"ok"},{"new_string":"BAD"}]}}`, "edits[1].new_string"},
		{"case insensitive", `{"TOOL_NAME":"Write","TOOL_INPUT":{"CONTENT":"BAD"}}`, "content"},
		{"invalid UTF-8", "{\"tool_name\":\"Write\",\"tool_input\":{\"content\":\"\xff\"}}", "content"},
		{"unpaired surrogate", `{"tool_name":"Write","tool_input":{"content":"\ud800"}}`, "content"},
		{"clean", `{"tool_name":"Write","tool_input":{"content":"<日本語>&"}}`, ""},
		{"null content", `{"tool_name":"Write","tool_input":{"content":null}}`, ""},
		{"null raw input", `{"tool_name":"Write","tool_input":null}`, ""},
		{"missing raw input", `{"tool_name":"Write"}`, ""},
		{"nil edits", `{"tool_name":"MultiEdit","tool_input":{"edits":null}}`, ""},
		{"empty edits", `{"tool_name":"MultiEdit","tool_input":{"edits":[]}}`, ""},
		{"unknown fields", `{"future":{"nested":true},"tool_name":"Write","tool_input":{"content":"ok"}}`, ""},
		{"invalid JSON", `{`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Keep the replacement rune out of the source; the repository's own hook rejects it.
			payload := strings.ReplaceAll(tt.input, "BAD", string(rune(0xfffd)))
			var out, diagnostic strings.Builder
			run(strings.NewReader(payload), &out, &diagnostic)
			if tt.field == "" {
				if out.Len() != 0 || diagnostic.Len() != 0 {
					t.Fatalf("unexpected output: %q, %q", out.String(), diagnostic.String())
				}
				return
			}
			if strings.Count(out.String(), "\n") != 1 || !strings.HasSuffix(out.String(), "\n") {
				t.Fatalf("expected one newline-terminated JSON value: %q", out.String())
			}
			var got hookOutput
			if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
				t.Fatal(err)
			}
			hook := got.HookSpecificOutput
			if hook.HookEventName != "PreToolUse" || hook.PermissionDecision != "deny" || !strings.Contains(hook.PermissionDecisionReason, tt.field) {
				t.Errorf("unexpected hook response: %+v", hook)
			}
			if !strings.Contains(diagnostic.String(), tt.field) {
				t.Errorf("missing diagnostic for %q: %q", tt.field, diagnostic.String())
			}
		})
	}
}
