package claudehooks

import (
	"os"
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
				Pattern:       `(^|\s)python3?(\s|$)`,
				Reason:        "python/python3 の直接実行は禁止",
				SystemMessage: "uv run を使ってください。",
			},
		},
	}
	input := HookInput{
		ToolName: "Bash",
		ToolInput: HookToolInput{
			Command: "python script.py",
		},
	}

	decision := EvaluatePreTool(cfg, input)
	if decision == nil {
		t.Fatal("expected deny decision")
	}
	if decision.SystemMessage == "" {
		t.Fatal("expected system message")
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
