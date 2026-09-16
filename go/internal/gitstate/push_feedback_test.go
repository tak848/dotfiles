package gitstate

import (
	"context"
	"strings"
	"testing"
)

func TestNormalPushModes(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"implicit":           nil,
		"explicit":           {"origin", "topic"},
		"quiet":              {"--quiet", "origin", "topic"},
		"quiet after remote": {"origin", "topic", "-q"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := newLocalGit(t)
			g.must(g.work, "config", "push.autoSetupRemote", "true")
			before := g.must(g.bare, "show-ref")
			c := Client{Runner: func(ctx context.Context, dir, program string, argv ...string) (string, error) {
				out, err := g.runner(ctx, dir, program, argv...)
				if program == "git" && argv[0] == "push" && err == nil {
					updates, parseErr := parsePorcelain(out)
					if parseErr != nil || len(updates) == 0 {
						t.Fatal("quiet must not hide a branch creation", out, parseErr)
					}
				}
				return out, err
			}}
			if err := c.CheckPush(context.Background(), g.work, args); err != nil {
				t.Fatal(err)
			}
			if after := g.must(g.bare, "show-ref"); after != before {
				t.Fatal("probe changed remote refs")
			}
		})
	}
}

func TestNativePushFailuresAreNotAuthenticationErrors(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"missing source":   {"origin", "missing-source"},
		"missing upstream": nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := newLocalGit(t)
			g.must(g.work, "config", "push.default", "simple")
			g.must(g.work, "config", "push.autoSetupRemote", "false")
			err := (Client{Runner: g.runner}).CheckPush(context.Background(), g.work, args)
			want := "送信元の ref"
			if name == "missing upstream" {
				want = "upstream が設定されていない"
			}
			if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "認証・通信") {
				t.Fatal(err)
			}
		})
	}
}

func TestNonFastForwardIsReportedAsGitRejection(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	base := strings.TrimSpace(g.must(g.work, "rev-parse", "HEAD"))
	g.must(g.work, "commit", "--allow-empty", "-m", "local work")
	g.must(g.bare, "config", "user.email", "test@example.invalid")
	g.must(g.bare, "config", "user.name", "Test")
	tree := strings.TrimSpace(g.must(g.bare, "rev-parse", "HEAD^{tree}"))
	remote := strings.TrimSpace(g.must(g.bare, "commit-tree", tree, "-p", base, "-m", "remote work"))
	g.must(g.bare, "update-ref", "refs/heads/topic", remote)
	before := g.must(g.bare, "show-ref")
	err := (Client{Runner: g.runner}).CheckPush(context.Background(), g.work, []string{"origin", "topic"})
	if err == nil || !strings.Contains(err.Error(), "取り込んでいない更新") {
		t.Fatal(err)
	}
	if after := g.must(g.bare, "show-ref"); after != before {
		t.Fatal("probe changed remote refs")
	}
}

func TestPushFailureClassificationDoesNotExposeStderr(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ stderr, stdout, want string }{
		{"error: src refspec secret does not match any", "", "送信元の ref"},
		{"fatal: The current branch secret has no upstream branch.", "", "upstream が設定されていない"},
		{"fatal: The upstream branch of your current branch does not match secret", "", "名前が一致しない"},
		{"fatal: No configured push destination. secret", "", "送信先が設定されていない"},
		{"https://user:secret@example.invalid", "!\trefs/heads/a:refs/heads/a\t[rejected] (non-fast-forward)\n", "取り込んでいない更新"},
		{"secret", "!\trefs/heads/a:refs/heads/a\t[rejected] (fetch first)\n", "取り込んでいない更新"},
	} {
		err := classifyPushError(exitError(1), tc.stdout, tc.stderr)
		if !exitIs(err, 1) {
			t.Fatal("lost exit code")
		}
		message := pushFailure("push-probe", err, "送信元と送信先を確認してください。").Error()
		if !strings.Contains(message, tc.want) || strings.Contains(message, "secret") || strings.Contains(message, "private URL") {
			t.Fatal(message)
		}
	}
}

func TestEmptySuccessfulPushOutput(t *testing.T) {
	t.Parallel()
	for _, out := range []string{"Done\n", "To remote\nDone\n"} {
		updates, err := parsePorcelain(out)
		if err != nil || len(updates) != 0 {
			t.Fatal(updates, err)
		}
	}
}

func TestPushErrorsIdentifyTheOperation(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"worktree", "local-branch", "push-destination", "push-probe", "push-output", "remote-ref", "github-repository", "github-pulls"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.live = ""
			c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
				cmd := strings.Join(args, " ")
				if stage == "push-output" && args[0] == "push" {
					return "invalid output with secret", nil
				}
				fail := stage == "worktree" && args[0] == "rev-parse" ||
					stage == "local-branch" && args[0] == "symbolic-ref" ||
					stage == "push-destination" && cmd == "remote" ||
					stage == "push-probe" && args[0] == "push" ||
					stage == "remote-ref" && args[0] == "ls-remote" ||
					stage == "github-repository" && name == "gh" && args[len(args)-1] == "repos/tak848/project" ||
					stage == "github-pulls" && name == "gh" && strings.Contains(cmd, "/pulls?")
				if fail {
					return "", exitError(127)
				}
				return f.runner(ctx, dir, name, args...)
			}}
			err := c.CheckPush(context.Background(), t.TempDir(), []string{"origin", "topic"})
			if err == nil {
				t.Fatal("failure accepted")
			}
			if strings.Contains(err.Error(), "認証・通信・送信先設定") || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private URL") {
				t.Fatal(err)
			}
		})
	}
}
