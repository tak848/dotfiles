package claudehooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	t.Parallel()

	got := ExpandPath("../x", "/tmp/repo/worktree")
	if got != "/tmp/repo/x" {
		t.Fatalf("unexpected expanded path: %s", got)
	}
}

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

func TestLocalPermissionDecision(t *testing.T) {
	t.Parallel()

	decision, ok := localPermissionDecision(HookInput{
		ToolName: "Bash",
		ToolInput: map[string]any{
			"command": "git push --force origin branch",
		},
	})
	if !ok {
		t.Fatal("expected local decision")
	}
	if decision.Behavior != "deny" {
		t.Fatalf("unexpected behavior: %s", decision.Behavior)
	}
}

func TestLocalPermissionDecisionRejectsCompoundCommands(t *testing.T) {
	t.Parallel()

	_, ok := localPermissionDecision(HookInput{
		ToolName: "Bash",
		ToolInput: map[string]any{
			"command": "git diff; rm -rf /tmp/x",
		},
	})
	if ok {
		t.Fatal("expected fallthrough for compound command")
	}
}

func TestEvaluatePreToolOutsideTrustedPath(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	input := HookInput{
		Cwd:      "/tmp/repo/worktree",
		ToolName: "Read",
		ToolInput: map[string]any{
			"file_path": "../other/file.txt",
		},
	}

	decision := EvaluatePreTool(cfg, input)
	if decision == nil {
		t.Fatal("expected deny decision")
	}
	if decision.Reason == "" {
		t.Fatal("expected deny reason")
	}
}

func TestExtractBashPathsSupportsInlineFlags(t *testing.T) {
	t.Parallel()

	got := extractBashPaths("/tmp/repo/worktree", `git -C../other status --file=/tmp/x`)
	if len(got) < 2 {
		t.Fatalf("expected multiple extracted paths, got %v", got)
	}
}

func TestMergeConfigFileReplacesTrustedPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "permission-gate.json")
	if err := os.WriteFile(path, []byte(`{"trusted_paths":["~/override"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	if err := mergeConfigFile(path, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedPaths) != 1 || cfg.TrustedPaths[0] != "~/override" {
		t.Fatalf("unexpected trusted_paths: %#v", cfg.TrustedPaths)
	}
}
