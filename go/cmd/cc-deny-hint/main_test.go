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
		{"process substitution as redirect target", "make > >(npm run log)", []string{"npm"}},
		{"env option value", "env -u FOO npm test", []string{"npm"}},
		{"env split string", "env -S 'npm test'", []string{"npm"}},
		{"env split string with quotes", `env -S 'sh -c "npm test"'`, []string{"npm"}},
		{"comment with paren in process substitution", "cat <( # note (\nnpm test\n)", []string{"npm"}},
		{"comment with paren in substitution", "echo $( # (\nnpm ls\n)", []string{"npm"}},
		{"$# in substitution", "echo $(echo $#; npm ls)", []string{"npm"}},
		{"escaped space before # in array", `tools=(foo\ #bar); npm test`, []string{"npm"}},
		{"escaped space before # in substitution", `echo $(printf '%s' foo\ #bar); npm test`, []string{"npm"}},
		{"heredoc with paren in process substitution", "cat <(cat <<EOF\n(\nEOF\nnpm test\n)", []string{"npm"}},
		{"heredoc with paren in substitution", "git commit -m \"$(cat <<'EOF'\nfix (x\nEOF\n)\" && npm test", []string{"npm"}},
		{"arithmetic shift", "x=$((1 << 2))\nnpm test", []string{"npm"}},
		{"arithmetic shift in process substitution", "cat <(\n echo $((1 << 2))\n npm test\n)", []string{"npm"}},
		{"line continuation before comment in process substitution", "cat <( \\\n# note (\nnpm test\n)", []string{"npm"}},
		{"line continuation before comment in array", "tools=(a \\\n# note (\n)\nnpm test", []string{"npm"}},
		{"line continuation in heredoc delimiter in process substitution", "cat <(cat <<\\\nEOF\nx\nEOF\nnpm test\n)", []string{"npm"}},
		{"arithmetic command shift in substitution", "echo $(\n(( x = 1 << 2 ))\nnpm ls\n)", []string{"npm"}},
		{"nice adjustment", "nice -n 10 npm test", []string{"npm"}},
		{"sudo long option with equals", "sudo --user=npm npm test", []string{"npm"}},
		{"time -p", "time -p npm test", []string{"npm"}},
		{"command -p", "command -p npm test", []string{"npm"}},
		{"case clause", "case foo in npm|pnpm) echo ok;; *) npm test;; esac", []string{"npm"}},
		{"after esac", "case foo in a) echo;; esac; npx foo", []string{"npx"}},
		{"last clause without ;;", "case foo in\nnpm) echo ok\nesac\nnpx foo", []string{"npx"}},
		{"substitution in case word", "case $(npm -v) in a) echo;; esac", []string{"npm"}},
		{"substitution in arithmetic", "echo $(( $(npm ls | wc -l) + 1 ))", []string{"npm"}},
		{"subshell in substitution", "echo $( (npm test); echo x)", []string{"npm"}},
		{"case in substitution", "echo $(case x in a) npm test;; esac)", []string{"npm"}},
		{"arithmetic for", "for ((i=0; i<3; i++)); do npm test; done", []string{"npm"}},
		{"nested subshells", "( (cd x) && npm test )", []string{"npm"}},
		{"substitution in array", `tools=(a "$(npx foo)")`, []string{"npx"}},
		{"substitution in heredoc", "cat <<EOF\n$(npm test)\nEOF", []string{"npm"}},
		{"backticks in heredoc", "cat <<EOF\n`npx foo`\nEOF", []string{"npx"}},
		{"quotes in heredoc are literal", "cat <<EOF\n'$(npm test)'\nEOF", []string{"npm"}},

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
		{"escaped substitution in heredoc", "cat <<EOF\n\\$(npm test)\nEOF", nil},
		{"double quoted delimiter", "cat <<\"EOF\"\n$(npm test)\nEOF", nil},
		{"partly quoted delimiter", "cat <<E\\OF\n$(npm test)\nEOF", nil},
		{"env unset value", "env -u npm echo ok", nil},
		{"sudo user value", "sudo -u npm echo ok", nil},
		{"sudo combined short options", "sudo -iu npm echo ok", nil},
		{"sudo long option value", "sudo --user npm echo ok", nil},
		{"exec argv0", "exec -a npm echo ok", nil},
		{"command -v", "command -v npm", nil},
		{"command -V", "command -V python3", nil},
		{"case label", "case foo in\nnpm) echo ok;;\nesac", nil},
		{"case label in parens", "case foo in (npm) echo ok;; esac", nil},
		{"case fallthrough", "case foo in a) echo;& npm) echo ok;;& *) echo;; esac", nil},
		{"quoted esac pattern", `case x in "esac") :;; npm) :;; esac`, nil},
		{"env split string quoted value", `env -S '-u "FOO npm" echo ok'`, nil},
		{"arithmetic expansion", "npm=2; echo $((npm + 1))", nil},
		{"arithmetic in double quotes", `echo "$((npm + 1))"`, nil},
		{"arithmetic command", "(( npm + 1 ))", nil},
		{"arithmetic condition", "if (( npm > 1 )); then echo; fi", nil},
		{"array assignment", "tools=(npm npx)", nil},
		{"array append", "tools+=(npm)", nil},
		{"multi-line array", "tools=(\n  npm # comment\n  npx\n)\necho ok", nil},
		{"process substitution then argument", "diff <(printf x) python3", nil},

		{"case pattern in substitution", "echo $(case x in npm) echo;; esac)", nil},
		{"escaped command name", `np\m install`, []string{"npm"}},
		// POSIX は "$(" と "(" を空白で分けるよう求めており、くっつけると算術式として読まれて解析できない
		{"unspaced subshell in substitution", "echo $((npm test); echo x)", nil},

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
