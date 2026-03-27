package claudehooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestShellSplit(t *testing.T) {
	t.Parallel()

	got := shellSplit(`git -C "../other repo" status`)
	if len(got) != 4 {
		t.Fatalf("unexpected token count: %d", len(got))
	}
	if got[2] != "../other repo" {
		t.Fatalf("unexpected token: %q", got[2])
	}
}

func TestEvaluatePreToolRule(t *testing.T) {
	t.Parallel()

	cfg := Config{
		PreToolDeny: []PreToolRule{
			{
				Matcher:       "Bash",
				Pattern:       `^([A-Za-z_][A-Za-z0-9_]*=\S+\s+)*python3?(\s|$)`,
				Reason:        "python/python3 の直接実行は禁止",
				SystemMessage: "uv run を使ってください。",
			},
		},
	}
	var input HookInput
	if err := json.Unmarshal([]byte(`{
		"tool_name":"Bash",
		"tool_input":{"command":"python script.py"}
	}`), &input); err != nil {
		t.Fatal(err)
	}

	decision := EvaluatePreTool(cfg, input)
	if decision == nil {
		t.Fatal("expected deny decision")
	}
	if decision.SystemMessage == "" {
		t.Fatal("expected system message")
	}
}

func TestEvaluatePreToolRuleDoesNotBlockUvRunPython(t *testing.T) {
	t.Parallel()

	cfg := Config{
		PreToolDeny: []PreToolRule{
			{
				Matcher:       "Bash",
				Pattern:       `^([A-Za-z_][A-Za-z0-9_]*=\S+\s+)*python3?(\s|$)`,
				Reason:        "python/python3 の直接実行は禁止",
				SystemMessage: "uv run を使ってください。",
			},
		},
	}
	var input HookInput
	if err := json.Unmarshal([]byte(`{
		"tool_name":"Bash",
		"tool_input":{"command":"uv run python script.py"}
	}`), &input); err != nil {
		t.Fatal(err)
	}

	if decision := EvaluatePreTool(cfg, input); decision != nil {
		t.Fatalf("expected uv run python to pass, got %+v", *decision)
	}
}

func TestExtractBashPathsSupportsInlineFlags(t *testing.T) {
	t.Parallel()

	got := extractBashPaths("/tmp/repo/worktree", `git -C../other status --file=/tmp/x`)
	if len(got) < 2 {
		t.Fatalf("expected multiple extracted paths, got %v", got)
	}
}

func TestMergeConfigFileAppendsRules(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "permission-gate.local.jsonnet")
	if err := os.WriteFile(path, []byte(`{ pre_tool_deny: [{ matcher: 'Bash', pattern: 'npx', reason: 'npx 禁止' }] }`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	if err := mergeConfigFile(path, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.PreToolDeny) != 1 {
		t.Fatalf("unexpected rule count: %d", len(cfg.PreToolDeny))
	}
}

func TestProjectLocalConfigPaths(t *testing.T) {
	t.Parallel()

	got := projectLocalConfigPaths("/tmp/repo/subdir")
	if len(got) != 2 {
		t.Fatalf("unexpected path count: %d", len(got))
	}
	if got[0] != "/tmp/repo/subdir/permission-gate.local.jsonnet" {
		t.Fatalf("unexpected first path: %s", got[0])
	}
	if got[1] != "/tmp/repo/subdir/.claude/permission-gate.local.jsonnet" {
		t.Fatalf("unexpected second path: %s", got[1])
	}
}

func TestSafeProjectLocalConfigPathsSkipsTrackedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "permission-gate.local.jsonnet"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Fatal("unexpected git directory")
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("add", "permission-gate.local.jsonnet")

	got := safeProjectLocalConfigPaths(dir)
	if len(got) != 0 {
		t.Fatalf("expected tracked project override to be skipped, got %v", got)
	}
}

func TestIsTrackedProjectFileFailsClosedOnGitError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "permission-gate.local.jsonnet")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	tracked, err := isTrackedProjectFile(dir, path)
	if err == nil {
		t.Fatal("expected git error")
	}
	if tracked {
		t.Fatal("expected non-tracked result on git error")
	}
}
