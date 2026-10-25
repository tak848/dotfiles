package main

import (
	"bytes"
	"encoding/json/v2"
	"slices"
	"strings"
	"testing"
)

var testRules = []rule{
	{Pattern: "git commit*--amend*", Reason: "amend"},
	{Pattern: "git -c commit.gpgsign=false*", Reason: "gpgsign"},
	{Pattern: "npm*", Reason: "npm"},
	{Pattern: "npx*", Reason: "npx"},
	{Pattern: "pip *", Reason: "pip"},
	{Pattern: "python*", Reason: "python"},
	{Pattern: "python3*", Reason: "python3"},
}

func runHints(t *testing.T, command string) []string {
	t.Helper()
	rules, err := json.Marshal(testRules)
	if err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": command}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{string(rules)}, bytes.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		return nil
	}
	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v: %s", err, out.String())
	}
	if got.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Fatalf("hookEventName = %q", got.HookSpecificOutput.HookEventName)
	}
	var reasons []string
	for _, line := range strings.Split(got.HookSpecificOutput.AdditionalContext, "\n") {
		reason, ok := strings.CutPrefix(line, hintPrefix)
		if !ok {
			t.Fatalf("line without prefix: %q", line)
		}
		reasons = append(reasons, reason)
	}
	return reasons
}

func TestHints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, command string
		want          []string
	}{
		// hooks の if で全ルールが発火していたもの
		{"command substitution", "echo $(date)", nil},
		{"variable", `for d in a b; do echo "$d"; done`, nil},
		{"backticks", "echo `date`", nil},

		{"amend", "git commit --amend -m x", []string{"amend"}},
		{"gpgsign", "git -c commit.gpgsign=false commit -m x", []string{"gpgsign"}},
		{"assignment prefix", "FOO=1 npm install", []string{"npm"}},
		{"after and", "ls && npx foo", []string{"npx"}},
		{"after pipe", "cat x | python3 -m json.tool", []string{"python", "python3"}},
		{"after semicolon", "cd x; npm test", []string{"npm"}},
		{"subshell", "( cd x && npm test )", []string{"npm"}},
		{"if condition", "if npm test; then echo ok; fi", []string{"npm"}},
		{"wrapper", "env FOO=1 npm test", []string{"npm"}},
		{"nested substitution", "echo $(npm ls)", []string{"npm"}},
		{"substitution in double quotes", `echo "$(npx foo)"`, []string{"npx"}},
		{"nested backticks", "echo `pip install x`", []string{"pip"}},
		{"bash -c", "bash -c 'pip install x'", []string{"pip"}},
		{"eval", "eval npm test", []string{"npm"}},
		{"process substitution", "diff <(npm ls) x", []string{"npm"}},

		{"quoted argument", `echo "npm install"`, nil},
		{"single quoted argument", "grep 'npx foo' file", nil},
		{"not a prefix", "uv run python x.py", nil},
		{"space required", "pipx install foo", nil},
		{"redirect target", "echo hi > npm", nil},
		{"fd redirect", "make 2>&1 | tee log", nil},
		{"comment", "ls # npm install", nil},
		{"heredoc body", "cat <<'EOF'\nnpm install\nEOF\necho done", nil},
		{"heredoc then command", "cat <<EOF > f\nx\nEOF\nnpm test", []string{"npm"}},
		{"heredoc with tabs", "cat <<-EOF\n\tnpm install\n\tEOF\n", nil},
		{"here-string", "cat <<< 'npm install'", nil},
		{"heredoc in substitution", "git commit -m \"$(cat <<'EOF'\nfix: npm\nEOF\n)\"", nil},

		{"unbalanced quote", `echo "npm install`, nil},
		{"unbalanced substitution", "echo $(npm ls", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := runHints(t, tt.command); !slices.Equal(got, tt.want) {
				t.Errorf("hints(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestIgnoresOtherInput(t *testing.T) {
	t.Parallel()
	rules, err := json.Marshal(testRules)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		args  []string
		input string
	}{
		{"no rules", nil, `{"tool_name":"Bash","tool_input":{"command":"npm test"}}`},
		{"invalid rules", []string{"not json"}, `{"tool_name":"Bash","tool_input":{"command":"npm test"}}`},
		{"other tool", []string{string(rules)}, `{"tool_name":"Read","tool_input":{"file_path":"npm"}}`},
		{"no command", []string{string(rules)}, `{"tool_name":"Bash","tool_input":{}}`},
		{"invalid input", []string{string(rules)}, `{`},
		{"too large", []string{string(rules)}, `{"tool_name":"Bash","tool_input":{"command":"npm ` + strings.Repeat("x", maxInput) + `"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if err := run(tt.args, strings.NewReader(tt.input), &out); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 {
				t.Errorf("output = %s, want none", out.String())
			}
		})
	}
}
