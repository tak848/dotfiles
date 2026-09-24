package gitstate

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type exitError int

func (e exitError) Error() string { return "private URL or stderr must never escape" }
func (e exitError) ExitCode() int { return int(e) }

type fixture struct {
	branch, head, status, live, remoteURL string
	config                                map[string]string
	prs                                   map[string][]pull
	parent                                string
	apiErr, liveErr                       error
	calls                                 []string
}

func TestExitIs(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		err  error
		code int
		want bool
	}{
		"nil":            {nil, 0, false},
		"direct":         {exitError(128), 128, true},
		"wrapped":        {fmt.Errorf("wrapped: %w", exitError(128)), 128, true},
		"joined":         {errors.Join(errors.New("other"), exitError(1)), 1, true},
		"different code": {exitError(1), 128, false},
		"other error":    {errors.New("other"), 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := exitIs(tc.err, tc.code); got != tc.want {
				t.Fatalf("exitIs=%v, want %v", got, tc.want)
			}
		})
	}
}

func newFixture() *fixture {
	return &fixture{branch: "topic", head: shaA, live: shaA, remoteURL: "https://github.com/tak848/project.git", config: map[string]string{}, prs: map[string][]pull{}}
}
func (f *fixture) runner(ctx context.Context, dir, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if name == "gh" {
		if f.apiErr != nil {
			return "", f.apiErr
		}
		endpoint := args[len(args)-1]
		if strings.Contains(endpoint, "/pulls?") {
			base := strings.Split(strings.TrimPrefix(endpoint, "repos/"), "/pulls?")[0]
			ps := f.prs[base]
			if ps == nil {
				ps = []pull{}
			}
			b, _ := json.Marshal(ps)
			return string(b), nil
		}
		repo := strings.TrimPrefix(endpoint, "repos/")
		r := repoInfo{FullName: repo, DefaultBranch: "main"}
		if f.parent != "" {
			r.Fork = true
			r.Parent = &repoInfo{FullName: f.parent}
		}
		b, _ := json.Marshal(r)
		return string(b), nil
	}
	if name != "git" {
		return "", fmt.Errorf("unexpected process")
	}
	switch strings.Join(args, " ") {
	case "rev-parse --is-inside-work-tree":
		return "true\n", nil
	case "status --porcelain=v1 --untracked-files=normal":
		return f.status, nil
	case "symbolic-ref --quiet --short HEAD":
		if f.branch == "" {
			return "", exitError(1)
		}
		return f.branch + "\n", nil
	case "rev-parse --verify HEAD":
		return f.head + "\n", nil
	case "remote":
		return "origin\n", nil
	case "remote get-url --push --all origin":
		return f.remoteURL + "\n", nil
	}
	if len(args) >= 3 && args[0] == "config" {
		if s, ok := f.config[args[2]]; ok {
			return s + "\n", nil
		}
		return "", exitError(1)
	}
	if len(args) > 0 && args[0] == "ls-remote" {
		if f.liveErr != nil {
			return "", f.liveErr
		}
		if f.live == "" {
			return "", exitError(2)
		}
		return f.live + "\t" + args[len(args)-1] + "\n", nil
	}
	if len(args) > 0 && args[0] == "push" {
		return "To " + f.remoteURL + "\n*\trefs/heads/topic:refs/heads/topic\t[new branch]\nDone\n", nil
	}
	if len(args) > 0 && args[0] == "merge-base" {
		return "", exitError(1)
	}
	return "", fmt.Errorf("unexpected git arguments %q", args)
}
func makePull(repo, base, branch, sha, state string, merged bool) pull {
	var p pull
	p.State = state
	p.Head.Ref = branch
	p.Head.SHA = sha
	p.Head.Repo = &repoInfo{FullName: repo}
	p.Base.Repo = &repoInfo{FullName: base}
	if merged {
		s := "2026-01-01T00:00:00Z"
		p.MergedAt = &s
	}
	return p
}
func TestStopStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(*fixture)
		owners    []string
		want, not string
	}{
		{"missing PR", nil, []string{"tak848"}, "PR がありません", ""},
		{"empty owners", nil, []string{}, "", ""},
		{"dirty", func(f *fixture) { f.status = " M existing.txt\n" }, nil, "未 commit", ""},
		{"detached", func(f *fixture) { f.branch = "" }, nil, "detached HEAD", ""},
		{"unpushed", func(f *fixture) { f.live = shaB }, nil, "反映されていません", ""},
		{"missing ref", func(f *fixture) { f.live = "" }, nil, "反映されていません", ""},
		{"api failure even owners empty", func(f *fixture) { f.apiErr = exitError(1) }, nil, "確認できません", "private URL"},
		{"transport failure", func(f *fixture) { f.liveErr = exitError(128) }, nil, "確認できません", "反映されていません"},
		{"merged deleted", func(f *fixture) {
			f.live = ""
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaA, "closed", true)}
		}, []string{"tak848"}, "", ""},
		{"merged new commit deleted", func(f *fixture) {
			f.live = ""
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "closed", true)}
		}, nil, "新しい作業ブランチ", "guard の確認を通して push"},
		{"merged new commit live", func(f *fixture) {
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "closed", true)}
		}, []string{"tak848"}, "PR がありません", ""},
		{"open current", func(f *fixture) {
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaA, "open", false)}
		}, []string{" TAK848 "}, "", ""},
		{"stale open", func(f *fixture) {
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "open", false)}
		}, []string{"tak848"}, "状態が一致しません", "PR がありません"},
		{"foreign base fork", func(f *fixture) { f.parent = "external/project" }, []string{"tak848"}, "", ""},
		{"required parent fork", func(f *fixture) { f.parent = "external/project" }, []string{"external"}, "PR がありません", ""},
		{"actual open fork base", func(f *fixture) {
			f.parent = "external/project"
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "open", false)}
		}, []string{"tak848"}, "状態が一致しません", "PR がありません"},
		{"other head repo", func(f *fixture) {
			f.prs["tak848/project"] = []pull{makePull("other/project", "tak848/project", "topic", shaA, "open", false)}
		}, []string{"tak848"}, "PR がありません", ""},
		{"default ahead", func(f *fixture) { f.branch = "main"; f.live = shaB }, []string{"tak848"}, "main へ直接 push せず", "guard の確認"},
		{"default clean", func(f *fixture) { f.branch = "main" }, []string{"tak848"}, "", ""},
		{"upstream target name", func(f *fixture) {
			f.config["push.default"] = "upstream"
			f.config["branch.topic.remote"] = "origin"
			f.config["branch.topic.merge"] = "refs/heads/review"
			f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "review", shaA, "open", false)}
		}, []string{"tak848"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			if tt.setup != nil {
				tt.setup(f)
			}
			reasons := (Client{Runner: f.runner}).CheckStop(context.Background(), t.TempDir(), tt.owners)
			s := strings.Join(reasons, "\n")
			if tt.want == "" && len(reasons) != 0 || tt.want != "" && !strings.Contains(s, tt.want) {
				t.Fatalf("reasons=%q", reasons)
			}
			if tt.not != "" && strings.Contains(s, tt.not) {
				t.Fatalf("unexpected reason=%q", s)
			}
		})
	}
}
func TestPushMergedLiveChecks(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, live string
		merged     bool
		fail       error
		wantError  bool
	}{{"existing", shaA, true, nil, false}, {"new", "", false, nil, false}, {"resurrection", "", true, nil, true}, {"unknown", "", false, exitError(128), true}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.live = tt.live
			f.liveErr = tt.fail
			if tt.merged {
				f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "closed", true)}
			}
			e := (Client{Runner: f.runner}).CheckPush(context.Background(), t.TempDir(), []string{"origin", "topic"})
			if (e != nil) != tt.wantError {
				t.Fatalf("err=%v", e)
			}
			if errors.Is(e, ErrMergedBranch) != (tt.name == "resurrection") {
				t.Fatalf("confirmed resurrection must be distinguished from lookup failure: %v", e)
			}
			if e != nil && strings.Contains(e.Error(), "private URL") {
				t.Fatal("leaked failure")
			}
			for _, call := range f.calls {
				if strings.HasPrefix(call, "git push ") && !strings.Contains(call, "--dry-run --porcelain --no-verify --recurse-submodules=no") {
					t.Fatalf("unsafe probe %s", call)
				}
			}
		})
	}
}
func TestPaginationBound(t *testing.T) {
	t.Parallel()
	f := newFixture()
	for range 100 {
		f.prs["tak848/project"] = append(f.prs["tak848/project"], makePull("tak848/project", "tak848/project", "topic", shaA, "open", false))
	}
	if _, e := (Client{Runner: f.runner}).pulls(context.Background(), t.TempDir(), repoInfo{FullName: "tak848/project"}, "topic"); e == nil {
		t.Fatal("accepted incomplete history")
	}
	if len(f.calls) != 10 {
		t.Fatalf("pages=%d", len(f.calls))
	}
}
func TestInvalidPorcelain(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", "To secret\n", "*\trefs/heads/a:refs/heads/a\t[new branch]\nDone\n", "To secret\n!\trefs/heads/a:refs/heads/a\t[rejected]\nDone\n", "To secret\n*\tbad\tbad\nDone\n"} {
		if _, e := parsePorcelain(s); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
func TestValidatePush(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--exec=arbitrary"}, {"--receive-pack", "bad"}, {"--recurse-submodules=on-demand"}, {"--mirror"}, {"-o", "server-option"}, {"origin", "a\nb"}} {
		if _, e := validatePush(args); e == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}
func TestRemoteIdentity(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		url, repo string
		deny      bool
	}{{"git@github.com:tak848/repo.git", "tak848/repo", false}, {"https://user:secret@github.com/tak848/repo.git", "tak848/repo", false}, {"ssh://git@github.com/tak848/repo.git", "tak848/repo", false}, {"git@github-alias:tak848/repo.git", "", true}, {"helper::repo", "", true}, {"/local/repo", "", false}, {"https://gitlab.com/other/repo.git", "", false}, {"https://github.com/a/b/extra", "", true}} {
		r, e := identify(tt.url)
		if r != tt.repo || (e != nil) != tt.deny {
			t.Fatalf("identity for %q = %q, %v", tt.url, r, e)
		}
	}
}

// localGit はコマンド単位で設定を隔離する。プロセスの HOME・環境変数・ユーザー設定・t.TempDir 外のリポジトリは変更しない。
type localGit struct {
	t                *testing.T
	root, work, bare string
	env              []string
}

func newLocalGit(t *testing.T) *localGit {
	t.Helper()
	root := t.TempDir()
	g := &localGit{t: t, root: root, work: filepath.Join(root, "work"), bare: filepath.Join(root, "remote.git")}
	for _, s := range os.Environ() {
		k := strings.SplitN(s, "=", 2)[0]
		if !strings.HasPrefix(k, "GIT_") && k != "HOME" && k != "XDG_CONFIG_HOME" {
			g.env = append(g.env, s)
		}
	}
	g.env = append(g.env, "HOME="+root, "XDG_CONFIG_HOME="+root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	g.must(root, "init", "--bare", "--initial-branch=main", g.bare)
	g.must(root, "init", "--initial-branch=main", g.work)
	g.must(g.work, "config", "user.email", "test@example.invalid")
	g.must(g.work, "config", "user.name", "Test")
	g.must(g.work, "config", "commit.gpgSign", "false")
	g.must(g.work, "config", "push.gpgSign", "false")
	g.must(g.work, "config", "push.default", "current")
	g.must(g.work, "commit", "--allow-empty", "-m", "initial")
	g.must(g.work, "remote", "add", "origin", g.bare)
	// ローカルでも本番 push は行わず、bare 側の fetch でテスト用 ref を用意する。
	head := strings.TrimSpace(g.must(g.work, "rev-parse", "HEAD"))
	g.must(g.bare, "fetch", g.work, head+":refs/heads/main")
	g.must(g.work, "switch", "-c", "topic")
	return g
}
func (g *localGit) runner(ctx context.Context, dir, name string, args ...string) (string, error) {
	if name != "git" {
		return "", errors.New("network/API forbidden in local fixture")
	}
	if len(args) > 0 && args[0] == "push" && !contains(args, "--dry-run") {
		g.t.Fatal("real push forbidden")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = g.env
	out, e := cmd.Output()
	if len(args) > 0 && args[0] == "push" {
		if ee, ok := errors.AsType[*exec.ExitError](e); ok {
			e = classifyPushError(e, string(out), string(ee.Stderr))
		}
		g.t.Logf("probe: %q -> %q err=%v", args, string(out), e)
	}
	return string(out), e
}
func (g *localGit) must(dir string, args ...string) string {
	g.t.Helper()
	s, e := g.runner(context.Background(), dir, "git", args...)
	if e != nil {
		g.t.Fatalf("git %q: %v", args, e)
	}
	return s
}
func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
func TestLocalDryRunDoesNotMutate(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	hook := filepath.Join(g.work, ".git", "hooks", "pre-push")
	marker := filepath.Join(g.root, "hook-ran")
	if e := os.WriteFile(hook, []byte("#!/bin/sh\nprintf ran >'"+marker+"'\nexit 1\n"), 0700); e != nil {
		t.Fatal(e)
	}
	before := g.must(g.bare, "show-ref")
	c := Client{Runner: g.runner}
	if e := c.CheckPush(context.Background(), g.work, []string{"--verify", "-u", "origin", "topic"}); e != nil {
		t.Fatal(e)
	}
	if after := g.must(g.bare, "show-ref"); after != before {
		t.Fatal("remote refs changed")
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("pre-push executed")
	}
	if _, e := g.runner(context.Background(), g.work, "git", "config", "--get", "branch.topic.remote"); !exitIs(e, 1) {
		t.Fatal("probe set upstream")
	}
}
func TestLocalNoRemoteAndNonGit(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	c := Client{Runner: g.runner}
	if r := c.CheckStop(context.Background(), g.root, nil); len(r) != 0 {
		t.Fatal(r)
	}
	g.must(g.work, "remote", "remove", "origin")
	if e := os.WriteFile(filepath.Join(g.work, "new.txt"), []byte("new\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if r := c.CheckStop(context.Background(), g.work, nil); len(r) != 1 || !strings.Contains(r[0], "未 commit") {
		t.Fatal(r)
	}
}
func TestLocalDestinationPrecedenceAndRewrite(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	g.must(g.work, "remote", "add", "other", g.bare)
	g.must(g.work, "config", "remote.other.pushurl", "https://github.com/push/repo.git")
	g.must(g.work, "config", "branch.topic.remote", "origin")
	g.must(g.work, "config", "remote.pushDefault", "other")
	c := Client{Runner: g.runner}
	check := func(explicit, want string) {
		t.Helper()
		ds, e := c.destinations(context.Background(), g.work, "topic", explicit)
		if e != nil || len(ds) != 1 || ds[0].url != want {
			t.Fatalf("destinations=%v err=%v", ds, e)
		}
	}
	check("", "https://github.com/push/repo.git")
	g.must(g.work, "config", "branch.topic.pushRemote", "origin")
	check("", g.bare)
	check("other", "https://github.com/push/repo.git")
	g.must(g.work, "config", "url.https://github.com/rewrite/.pushInsteadOf", "example:")
	check("example:repo.git", "https://github.com/rewrite/repo.git")
	g.must(g.work, "config", "--add", "remote.other.pushurl", "https://github.com/second/repo.git")
	ds, e := c.destinations(context.Background(), g.work, "topic", "other")
	if e != nil || len(ds) != 2 {
		t.Fatalf("multiple push URLs=%v %v", ds, e)
	}
}
func TestLocalPrunedMergedBranch(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	g.must(g.work, "config", "branch.topic.remote", "origin")
	g.must(g.work, "config", "branch.topic.merge", "refs/heads/topic")
	c := Client{Runner: g.runner}
	if sha, e := c.live(context.Background(), g.work, g.bare, "refs/heads/topic"); e != nil || sha != "" {
		t.Fatalf("missing=%q %v", sha, e)
	}
	g.must(g.work, "fetch", "--prune", "origin")
	if r := c.CheckStop(context.Background(), g.work, nil); !strings.Contains(strings.Join(r, "\n"), "反映されていません") {
		t.Fatal(r)
	}
}
func TestLocalStopUpstreamDifferentName(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	g.must(g.work, "config", "push.default", "upstream")
	g.must(g.work, "config", "branch.topic.remote", "origin")
	g.must(g.work, "config", "branch.topic.merge", "refs/heads/review")
	head := strings.TrimSpace(g.must(g.work, "rev-parse", "HEAD"))
	g.must(g.bare, "fetch", g.work, head+":refs/heads/review")
	c := Client{Runner: g.runner}
	if r := c.CheckStop(context.Background(), g.work, nil); len(r) != 0 {
		t.Fatal(r)
	}
}
func TestLocalMultiplePushDestinations(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	other := filepath.Join(g.root, "other.git")
	g.must(g.root, "init", "--bare", other)
	g.must(g.work, "config", "--add", "remote.origin.pushurl", g.bare)
	g.must(g.work, "config", "--add", "remote.origin.pushurl", other)
	if e := (Client{Runner: g.runner}).CheckPush(context.Background(), g.work, []string{"origin", "topic"}); e != nil {
		t.Fatal(e)
	}
}
func TestAncestorAndMissingObject(t *testing.T) {
	t.Parallel()
	for _, branch := range []string{"topic", "main"} {
		for _, code := range []int{0, 128} {
			t.Run(fmt.Sprintf("%s-%d", branch, code), func(t *testing.T) {
				t.Parallel()
				f := newFixture()
				f.branch = branch
				f.live = shaB
				f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", branch, shaB, "open", false)}
				c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
					if name == "git" && args[0] == "merge-base" {
						if code == 0 {
							return "", nil
						}
						return "", exitError(code)
					}
					return f.runner(ctx, dir, name, args...)
				}}
				reasons := c.CheckStop(context.Background(), t.TempDir(), []string{"tak848"})
				s := strings.Join(reasons, "\n")
				if code == 0 && len(reasons) != 0 {
					t.Fatal(reasons)
				}
				if code == 128 && (!strings.Contains(s, "確認できません") || strings.Contains(s, "未反映") || strings.Contains(s, "反映されていません")) {
					t.Fatal(reasons)
				}
			})
		}
	}
}
func TestUnknownMergedObjectDoesNotInventAdditionalCommits(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.live = ""
	f.prs["tak848/project"] = []pull{makePull("tak848/project", "tak848/project", "topic", shaB, "closed", true)}
	c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
		if name == "git" && args[0] == "merge-base" {
			return "", exitError(128)
		}
		return f.runner(ctx, dir, name, args...)
	}}
	reasons := strings.Join(c.CheckStop(context.Background(), t.TempDir(), nil), "\n")
	if !strings.Contains(reasons, "確認できません") || strings.Contains(reasons, "追加 commit") {
		t.Fatal(reasons)
	}
}
func TestCustomSSHCommandIsUnknown(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.remoteURL = "git@github.com:tak848/project.git"
	f.config["core.sshCommand"] = "custom-ssh -i identity"
	e := (Client{Runner: f.runner}).CheckPush(context.Background(), t.TempDir(), []string{"origin", "topic"})
	if e == nil {
		t.Fatal("custom SSH accepted")
	}
	for _, s := range f.calls {
		if strings.HasPrefix(s, "git push ") || strings.HasPrefix(s, "git ls-remote ") {
			t.Fatal("network probe ran with different SSH configuration")
		}
	}
}
func TestLocalImplicitAndExplicitWithoutRemote(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	c := Client{Runner: g.runner}
	if e := c.CheckPush(context.Background(), g.work, nil); e != nil {
		t.Fatal(e)
	}
	g.must(g.work, "remote", "remove", "origin")
	if e := c.CheckPush(context.Background(), g.work, []string{g.bare, "HEAD:refs/heads/review"}); e != nil {
		t.Fatal(e)
	}
}
func TestLocalConfiguredRefspec(t *testing.T) {
	t.Parallel()
	g := newLocalGit(t)
	g.must(g.work, "config", "remote.origin.push", "refs/heads/topic:refs/heads/review")
	head := strings.TrimSpace(g.must(g.work, "rev-parse", "HEAD"))
	g.must(g.bare, "fetch", g.work, head+":refs/heads/review")
	c := Client{Runner: g.runner}
	if r := c.CheckStop(context.Background(), g.work, nil); len(r) != 0 {
		t.Fatal(r)
	}
	if e := c.CheckPush(context.Background(), g.work, nil); e != nil {
		t.Fatal(e)
	}
}
func TestPushForkParentMerged(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.live = ""
	f.parent = "external/project"
	f.prs["external/project"] = []pull{makePull("tak848/project", "external/project", "topic", shaA, "closed", true)}
	if e := (Client{Runner: f.runner}).CheckPush(context.Background(), t.TempDir(), []string{"origin", "topic"}); e == nil {
		t.Fatal("parent merged branch recreated")
	}
}
func TestPushFailureCases(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"auth", "malformed", "helper"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			f.live = ""
			if kind == "auth" {
				f.apiErr = exitError(1)
			}
			if kind == "helper" {
				f.config["remote.origin.vcs"] = "custom"
			}
			c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
				if kind == "malformed" && name == "git" && args[0] == "push" {
					return "success\n", nil
				}
				return f.runner(ctx, dir, name, args...)
			}}
			if e := c.CheckPush(context.Background(), t.TempDir(), []string{"origin", "topic"}); e == nil {
				t.Fatal("unknown state accepted")
			}
		})
	}
}
func TestOutputBound(t *testing.T) {
	t.Parallel()
	c := Client{Runner: func(context.Context, string, string, ...string) (string, error) {
		return strings.Repeat("x", maxOutput+1), nil
	}}
	if _, e := c.git(context.Background(), t.TempDir(), "remote"); e == nil {
		t.Fatal("unbounded output")
	}
}
func TestOwnersAreUnchanged(t *testing.T) {
	t.Parallel()
	owners := []string{" TAK848 "}
	copyOf := append([]string{}, owners...)
	_ = (Client{Runner: newFixture().runner}).CheckStop(context.Background(), t.TempDir(), owners)
	if !reflect.DeepEqual(owners, copyOf) {
		t.Fatal("mutated owners")
	}
}
