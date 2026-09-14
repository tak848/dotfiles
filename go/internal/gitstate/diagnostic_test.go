package gitstate

import (
	"context"
	"encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultBranchDoesNotDependOnGitHub(t *testing.T) {
	t.Parallel()
	for _, branch := range []string{"main", "master"} {
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.branch = branch
			f.remoteURL = "https://github.com/other-org/project.git"
			f.apiErr = exitError(1)
			c := Client{Runner: f.runner}
			// owner が対象でも、main と送信先が一致するなら PR は不要。
			if got := c.CheckStop(context.Background(), t.TempDir(), []string{"other-org"}); len(got) != 0 {
				t.Fatal(got)
			}
			for _, call := range f.calls {
				if strings.HasPrefix(call, "gh ") {
					t.Fatalf("unnecessary GitHub dependency: %s", call)
				}
			}
		})
	}
}

func TestDefaultBranchStillDetectsProblems(t *testing.T) {
	t.Parallel()
	for name, setup := range map[string]func(*fixture){
		"dirty": func(f *fixture) { f.status = " M file\n" },
		"ahead": func(f *fixture) { f.live = shaB },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.branch = "main"
			f.apiErr = exitError(1)
			setup(f)
			reasons := (Client{Runner: f.runner}).CheckStop(context.Background(), t.TempDir(), nil)
			if len(reasons) == 0 || IsCheckFailure(reasons[0]) {
				t.Fatalf("expected an observed problem, got %v", reasons)
			}
		})
	}
}

func TestFailureStages(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"worktree", "worktree-status", "local-branch", "push-destination", "push-ref", "github-repository", "remote-ref", "github-pulls"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
				cmd := strings.Join(args, " ")
				fail := stage == "worktree" && cmd == "rev-parse --is-inside-work-tree" ||
					stage == "worktree-status" && args[0] == "status" ||
					stage == "local-branch" && args[0] == "symbolic-ref" ||
					stage == "push-destination" && cmd == "remote" ||
					stage == "push-ref" && cmd == "config --get push.default" ||
					stage == "github-repository" && name == "gh" && args[len(args)-1] == "repos/tak848/project" ||
					stage == "remote-ref" && args[0] == "ls-remote" ||
					stage == "github-pulls" && name == "gh" && strings.Contains(cmd, "/pulls?")
				if fail {
					return "", exitError(127)
				}
				return f.runner(ctx, dir, name, args...)
			}}
			reasons := c.CheckStop(context.Background(), t.TempDir(), []string{"tak848"})
			if len(reasons) != 1 || !IsCheckFailure(reasons[0]) || !strings.Contains(reasons[0], "["+stage+"]") || !strings.Contains(reasons[0], "127") {
				t.Fatalf("missing stage/exit: %v", reasons)
			}
			if strings.Contains(reasons[0], "private URL") {
				t.Fatal("raw stderr leaked")
			}
		})
	}
}

func TestMissingRemoteCommitIsFetchedSilently(t *testing.T) {
	t.Parallel()
	for _, branch := range []string{"main", "topic"} {
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.branch, f.live = branch, shaB
			fetches := 0
			c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
				if name == "git" {
					switch args[0] {
					case "merge-base", "cat-file":
						if fetches == 0 {
							return "", exitError(128)
						}
						return "", nil
					case "fetch":
						fetches++
						if args[len(args)-1] != shaB || args[len(args)-2] != f.remoteURL {
							t.Fatalf("wrong fetch target: %v", args)
						}
						for _, flag := range []string{"--no-write-fetch-head", "--refmap=", "--no-tags", "--recurse-submodules=no", "--no-auto-maintenance", "--no-write-commit-graph"} {
							if !contains(args, flag) {
								t.Fatalf("missing %s: %v", flag, args)
							}
						}
						return "", nil
					}
				}
				return f.runner(ctx, dir, name, args...)
			}}
			dir := t.TempDir()
			for range 2 {
				if reasons := c.CheckStop(context.Background(), dir, nil); len(reasons) != 0 {
					t.Fatal(reasons)
				}
			}
			if fetches != 1 {
				t.Fatalf("fetches=%d, want 1", fetches)
			}
		})
	}
}

func TestMainRemoteAdvanceWithoutFetch(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	g.must(g.work, "switch", "main")
	g.must(g.work, "fetch", "origin")
	head := strings.TrimSpace(g.must(g.work, "rev-parse", "HEAD"))
	// bare 側だけに commit を作り、作業ツリーの origin/main を古いままにする。
	g.must(g.bare, "config", "user.email", "test@example.invalid")
	g.must(g.bare, "config", "user.name", "Test")
	tree := strings.TrimSpace(g.must(g.bare, "rev-parse", "HEAD^{tree}"))
	next := strings.TrimSpace(g.must(g.bare, "commit-tree", tree, "-p", head, "-m", "remote advance"))
	g.must(g.bare, "update-ref", "refs/heads/main", next)
	if got := strings.TrimSpace(g.must(g.work, "rev-parse", "origin/main")); got != head {
		t.Fatal("fixture already fetched remote advance")
	}
	if got := g.must(g.work, "status", "--porcelain"); got != "" {
		t.Fatal("fixture is dirty")
	}
	g.must(g.bare, "tag", "remote-only-tag", next)
	g.must(g.work, "config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/heads/unwanted/*")
	refs := g.must(g.work, "show-ref")
	remoteRefs := g.must(g.bare, "show-ref")
	fetchHeadPath := filepath.Join(g.work, ".git", "FETCH_HEAD")
	fetchHead, err := os.ReadFile(fetchHeadPath)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(g.work, ".git", "config")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	c := Client{Runner: g.runner}
	if got := c.CheckStop(context.Background(), g.work, nil); len(got) != 0 {
		t.Fatalf("automatic fetch should resolve silently: %v", got)
	}
	g.must(g.work, "cat-file", "-e", next+"^{commit}")
	if after := g.must(g.work, "show-ref"); after != refs {
		t.Fatal("local/tracking/tag refs changed")
	}
	if after := g.must(g.bare, "show-ref"); after != remoteRefs {
		t.Fatal("remote refs changed")
	}
	if after := strings.TrimSpace(g.must(g.work, "rev-parse", "HEAD")); after != head {
		t.Fatal("HEAD changed")
	}
	if after := g.must(g.work, "status", "--porcelain"); after != "" {
		t.Fatal("working tree changed")
	}
	if after, err := os.ReadFile(fetchHeadPath); err != nil || string(after) != string(fetchHead) {
		t.Fatal("FETCH_HEAD changed", err)
	}
	if after, err := os.ReadFile(configPath); err != nil || string(after) != string(config) {
		t.Fatal("config changed", err)
	}
}

func TestObjectFetchFailureAndRetryLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		fetchErr  error
		wantStage string
	}{
		{"transport", exitError(128), "[fetch-object]"},
		{"timeout", context.DeadlineExceeded, "[fetch-object]"},
		{"object still unavailable", nil, "[commit-graph]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fetches := 0
			c := Client{Runner: func(_ context.Context, _ string, _ string, args ...string) (string, error) {
				if args[0] == "fetch" {
					fetches++
					return "", tc.fetchErr
				}
				return "", exitError(128)
			}}
			covered, failure := c.covers(context.Background(), t.TempDir(), "https://user:secret@github.com/test/repo.git", shaA, shaB)
			if covered || fetches != 1 || !IsCheckFailure(failure) || !strings.Contains(failure, tc.wantStage) || strings.Contains(failure, "secret") || strings.Contains(failure, "private URL") {
				t.Fatal(covered, fetches, failure)
			}
			if len([]rune(failure)) > 130 {
				t.Fatal("fetch failure is too verbose", failure)
			}
		})
	}
}

func TestFetchedObjectCanStillBeDiverged(t *testing.T) {
	t.Parallel()
	fetched := false
	c := Client{Runner: func(_ context.Context, _ string, _ string, args ...string) (string, error) {
		if args[0] == "fetch" {
			fetched = true
			return "", nil
		}
		if args[0] == "merge-base" && fetched {
			return "", exitError(1)
		}
		return "", exitError(128)
	}}
	covered, failure := c.covers(context.Background(), t.TempDir(), "/remote.git", shaA, shaB)
	if covered || failure != "" || !fetched {
		t.Fatal(covered, failure, fetched)
	}
}

func TestCurrentPRDoesNotFetchOldHistory(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.prs["tak848/project"] = []pull{
		makePull("tak848/project", "tak848/project", "topic", shaB, "closed", true),
		makePull("tak848/project", "tak848/project", "topic", shaA, "open", false),
	}
	c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
		if name == "git" && (args[0] == "fetch" || args[0] == "merge-base") {
			t.Fatal("irrelevant history inspected")
		}
		return f.runner(ctx, dir, name, args...)
	}}
	if reasons := c.CheckStop(context.Background(), t.TempDir(), []string{"tak848"}); len(reasons) != 0 {
		t.Fatal(reasons)
	}
}

func TestStackedPRDoesNotProduceUnknown(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaA, "open", false)}
	c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
		s, err := f.runner(ctx, dir, name, args...)
		if err == nil && name == "gh" && strings.Contains(args[len(args)-1], "/pulls?") {
			var ps []map[string]any
			if err := json.Unmarshal([]byte(s), &ps); err != nil {
				t.Fatal(err)
			}
			for _, p := range ps {
				p["base"].(map[string]any)["ref"] = "another-open-pr-branch"
			}
			data, err := json.Marshal(ps)
			return string(data), err
		}
		return s, err
	}}
	if reasons := c.CheckStop(context.Background(), t.TempDir(), []string{"tak848"}); len(reasons) != 0 {
		t.Fatal(reasons)
	}
}

func TestDiagnosticDoesNotLeakError(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"validation": errors.New("https://user:secret@example.invalid"),
		"command":    &exec.Error{Name: "secret", Err: exec.ErrNotFound},
		"path":       &os.PathError{Op: "chdir", Path: "secret", Err: os.ErrPermission},
		"exit":       exitError(128),
		"timeout":    context.DeadlineExceeded,
		"cancel":     context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := checkFailure("test-stage", err, "対象を確認してください。")
			if !IsCheckFailure(r) || !strings.Contains(r, "[test-stage]") || strings.Contains(r, "secret") || strings.Contains(r, "private URL") {
				t.Fatal(r)
			}
		})
	}
}
