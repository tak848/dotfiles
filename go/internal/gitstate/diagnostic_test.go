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

func TestStopObservedProblemsAreNotCheckFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, want string
		setup      func(*fixture)
	}{
		{"detached HEAD", "detached HEAD", func(f *fixture) { f.branch = "" }},
		{"multiple open PR bases", "複数の base リポジトリ", func(f *fixture) {
			f.parent = "external/project"
			for _, base := range []string{"tak848/project", f.parent} {
				f.prs[base] = []pull{makePull("tak848/project", base, "topic", shaA, "open", false)}
			}
		}},
		{"stale PR head", "と一致しません", func(f *fixture) {
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "open", false)}
		}},
		{"dirty", "未 commit", func(f *fixture) { f.status = " M file\n" }},
		{"unpushed", "含まれていません", func(f *fixture) { f.live = shaB }},
		{"default ahead", "既定ブランチ", func(f *fixture) { f.branch, f.live = "main", shaB }},
		{"missing PR", "を含む PR はありません", func(f *fixture) { f.prs = nil }},
		{"merged branch with new commit", "マージ済み PR #12", func(f *fixture) {
			f.live = ""
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "closed", true)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaA, "open", false)}
			tc.setup(f)
			reasons := (Client{Runner: f.runner}).CheckStop(context.Background(), t.TempDir(), []string{"tak848"})
			if len(reasons) != 1 || !strings.Contains(reasons[0], tc.want) || IsCheckFailure(reasons[0]) {
				t.Fatalf("expected an observed problem containing %q, got %v", tc.want, reasons)
			}
		})
	}
}

func TestStopReasonsNameWhatWasChecked(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(*fixture)
		want  []string
	}{
		{"dirty", func(f *fixture) { f.status = " M a.go\n?? b.go\n M c.go\n" }, []string{"未 commit の変更が 3 件", "a.go, b.go, 他 1 件"}},
		{"detached", func(f *fixture) { f.branch = "" }, []string{"HEAD（aaaaaaa）", "detached HEAD"}},
		{"diverged", func(f *fixture) { f.live = shaB }, []string{"ブランチ topic の HEAD aaaaaaa", "push 先 origin の refs/heads/topic（bbbbbbb）", "履歴が分岐", "--force-with-lease"}},
		{"multiple bases", func(f *fixture) {
			f.parent = "external/project"
			for _, base := range []string{"tak848/project", f.parent} {
				f.prs[base] = []pull{makePull("tak848/project", base, "topic", shaA, "open", false)}
			}
		}, []string{"#12（base tak848/project）", "#12（base external/project）", "ユーザーに確認"}},
		{"stale PR head", func(f *fixture) {
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "open", false)}
		}, []string{"#12（base tak848/project）の head は bbbbbbb", "HEAD aaaaaaa"}},
		{"merged deleted", func(f *fixture) {
			f.live = ""
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "closed", true)}
		}, []string{"push 先 origin に refs/heads/topic がありません", "マージ済み PR #12（base tak848/project）", "HEAD aaaaaaa"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.remoteURL = "https://user:secret-token@github.com/tak848/project.git"
			tc.setup(f)
			s := strings.Join((Client{Runner: f.runner}).CheckStop(context.Background(), t.TempDir(), []string{"tak848"}), "\n")
			for _, want := range tc.want {
				if !strings.Contains(s, want) {
					t.Fatalf("missing %q in %q", want, s)
				}
			}
			if strings.Contains(s, "secret-token") {
				t.Fatal("push URL leaked")
			}
		})
	}
}

func TestUnpushedAheadUsesPlainPush(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.live = shaB
	c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
		// 送信先 shaB は HEAD の祖先、HEAD は送信先に含まれない。
		if name == "git" && args[0] == "merge-base" {
			if args[2] == shaB && args[3] == shaA {
				return "", nil
			}
			return "", exitError(1)
		}
		return f.runner(ctx, dir, name, args...)
	}}
	s := strings.Join(c.CheckStop(context.Background(), t.TempDir(), nil), "\n")
	if !strings.Contains(s, "通常の push で反映できます") || strings.Contains(s, "force") {
		t.Fatal(s)
	}
}

func TestStopBranchErrorClassification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		err         error
		wantFailure bool
	}{
		{"detached", exitError(1), false},
		{"wrapped detached", errors.Join(errors.New("private detail"), exitError(1)), false},
		{"fatal", exitError(128), true},
		{"timeout", context.DeadlineExceeded, true},
		{"canceled", context.Canceled, true},
		{"cannot execute", &exec.Error{Name: "secret", Err: exec.ErrNotFound}, true},
		{"cannot access directory", &os.PathError{Op: "chdir", Path: "secret", Err: os.ErrPermission}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.status = " M file\n"
			c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
				if name == "git" && args[0] == "symbolic-ref" {
					return "", tc.err
				}
				return f.runner(ctx, dir, name, args...)
			}}
			reasons := c.CheckStop(context.Background(), t.TempDir(), nil)
			if len(reasons) != 2 || IsCheckFailure(reasons[0]) || !strings.Contains(reasons[0], "未 commit") {
				t.Fatalf("lost observed worktree problem: %v", reasons)
			}
			if IsCheckFailure(reasons[1]) != tc.wantFailure {
				t.Fatalf("IsCheckFailure=%v, want %v: %v", IsCheckFailure(reasons[1]), tc.wantFailure, reasons)
			}
			want := "detached HEAD"
			if tc.wantFailure {
				want = "[local-branch]"
			}
			if !strings.Contains(reasons[1], want) || strings.Contains(reasons[1], "private") || strings.Contains(reasons[1], "secret") {
				t.Fatalf("incorrect diagnostic: %v", reasons)
			}
		})
	}
}

func TestFailureStages(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"worktree", "worktree-status", "local-branch", "push-destination", "local-head", "push-ref", "github-repository", "remote-default-branch", "remote-ref", "github-pulls"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			if stage == "remote-default-branch" {
				f.remoteURL = "/remote.git"
			}
			c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
				cmd := strings.Join(args, " ")
				fail := stage == "worktree" && cmd == "rev-parse --is-inside-work-tree" ||
					stage == "worktree-status" && args[0] == "status" ||
					stage == "local-branch" && args[0] == "symbolic-ref" ||
					stage == "push-destination" && cmd == "remote" ||
					stage == "local-head" && cmd == "rev-parse --verify HEAD" ||
					stage == "push-ref" && cmd == "config --get push.default" ||
					stage == "github-repository" && name == "gh" && args[len(args)-1] == "repos/tak848/project" ||
					stage == "remote-default-branch" && args[0] == "ls-remote" && args[1] == "--symref" ||
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

func TestFeedbackExplainsTheOperation(t *testing.T) {
	t.Parallel()
	raw := checkFailure("github-repository", exitError(403), "リポジトリの読み取り権限を確認してください。")
	feedback := Feedback(raw)
	for _, want := range []string{"GitHub のリポジトリ情報", "403", "読み取り権限"} {
		if !strings.Contains(feedback, want) {
			t.Fatal(feedback)
		}
	}
	for _, forbidden := range []string{CheckFailurePrefix, "[github-repository]", "hook", "private URL"} {
		if strings.Contains(feedback, forbidden) {
			t.Fatal(feedback)
		}
	}
	problem := "未 commit の変更があります。"
	if Feedback(problem) != problem || !IsCheckFailure(raw) {
		t.Fatal("presentation changed classification")
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
