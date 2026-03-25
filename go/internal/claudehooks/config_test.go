package claudehooks

import "testing"

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
