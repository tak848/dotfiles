package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/tak848/dotfiles/go/internal/gitstate"
)

func runForTest(ctx context.Context, r io.Reader, w io.Writer, check checkFunc) error {
	return runWithEnv(ctx, r, w, check, func(string) string { return "" })
}
func TestOptOutSkipsInputAndCheck(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0", "false", "OFF", " no "} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if e := runWithEnv(context.Background(), nil, nil, nil, func(string) string { return value }); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestFindPushes(t *testing.T) {
	t.Parallel()
	repo := []push{{dir: "/repo", args: []string{}}}
	topic := []push{{dir: "/repo", args: []string{"origin", "topic"}}}
	tests := []struct {
		name, command string
		want          []push
		unverifiable  bool
	}{
		{"plain", "git push", repo, false},
		{"trailing LF", "git push\n", repo, false},
		{"echo words", "echo git push", nil, false},
		{"log word", "git log --grep push", nil, false},
		{"eval words", "eval git push", repo, false},
		{"if compound", "if true; then git push; fi", repo, false},
		{"if condition", "if git push; then true; fi", repo, false},
		{"for compound", "for x in a; do git push; done", repo, false},
		{"while compound", "while true; do git push; done", repo, false},
		{"until compound", "until git push; do true; done", repo, false},
		{"elif compound", "if false; then true; elif git push; then true; fi", repo, false},
		{"else compound", "if false; then true; else git push; fi", repo, false},
		{"negation", "! git push", repo, false},
		{"case compound", "case a in a) git push;; esac", repo, false},
		{"brace compound", "{ git push; }", repo, false},
		{"function compound", "function f { git push; }; f", repo, false},
		{"reserved literal", "echo 'then git push'", nil, false},
		{"compound literal", "echo 'if true; then git push; fi'", nil, false},
		{"remote", "git push -u origin HEAD:refs/heads/topic", []push{{dir: "/repo", args: []string{"-u", "origin", "HEAD:refs/heads/topic"}}}, false},
		{"cd quoted", `cd '/work/a b' && git push origin 'topic'`, []push{{dir: "/work/a b", args: []string{"origin", "topic"}}}, false},
		{"cd relative", `cd ../other && git push`, []push{{dir: "/other", args: []string{}}}, false},
		{"cd end options", `cd -- other && git push`, []push{{dir: "/repo/other", args: []string{}}}, false},
		{"cd dynamic", `cd "$DIR" && git push`, nil, true},
		{"cd tilde", `cd ~/work && git push`, nil, true},
		{"C escaped", `git -C /work/a\ b push origin topic`, []push{{dir: "/work/a b", args: []string{"origin", "topic"}}}, false},
		{"multiple C", `git -C /work -C other push`, []push{{dir: "/work/other", args: []string{}}}, false},
		{"quoted dollar", `git -C '/work/$literal' push`, []push{{dir: "/work/$literal", args: []string{}}}, false},
		{"escaped dollar", `git -C /work/\$literal push`, []push{{dir: "/work/$literal", args: []string{}}}, false},
		{"dynamic", `git push "$REMOTE"`, nil, true},
		{"dynamic branch", `git push origin "$(git branch --show-current)"`, nil, true},
		{"dynamic command", `$GIT push`, nil, true},
		{"dynamic subcommand", `git $ACTION`, nil, true},
		{"glob", `git push origin refs/heads/*`, nil, true},
		{"quoted glob", `git push origin 'refs/heads/*:refs/heads/*'`, []push{{dir: "/repo", args: []string{"origin", "refs/heads/*:refs/heads/*"}}}, false},
		{"switch", `git switch topic && git push`, repo, false},
		{"chained commit", `git add x && git commit -m y && git push origin topic && git rev-parse HEAD`, topic, false},
		{"extra command", `git push; echo done`, repo, false},
		{"pipe and redirect", `git push origin topic 2>&1 | tail -2`, topic, false},
		{"two pushes", `git push origin topic; git push origin other`, []push{topic[0], {dir: "/repo", args: []string{"origin", "other"}}}, false},
		{"eval", `eval 'git push'`, repo, false},
		{"shell", `bash -c 'git push origin topic'`, topic, false},
		{"shell dynamic", `bash -c "git push $REMOTE"`, nil, true},
		{"subshell", `(git push)`, repo, false},
		{"substitution", `echo $(git push)`, repo, false},
		{"quoted substitution", `echo "$(git push)"`, repo, false},
		{"backtick", "echo `git push`", repo, false},
		{"environment", `FOO=bar git push`, repo, false},
		{"git environment", `GIT_DIR=/x git push`, nil, true},
		{"env wrapper", `env FOO=bar git push origin topic`, topic, false},
		{"env git environment", `env GIT_DIR=/x git push`, nil, true},
		{"env options", `env -u FOO git push`, nil, true},
		{"wrapper", `command git push`, repo, false},
		{"command lookup", `command -v git`, nil, false},
		{"redirect", `git push >output`, repo, false},
		{"git config", `git -c push.default=current push`, nil, true},
		{"git dir", `git --git-dir=/x push`, nil, true},
		{"unclosed", `git push 'origin`, nil, false},
		{"echo", `echo 'git push'`, nil, false},
		{"literal substitution", `echo '$(git push)'`, nil, false},
		{"log", `git log --grep='git push'`, nil, false},
		{"status", `git status`, nil, false},
		{"comment", `echo ok # git push`, nil, false},
		{"heredoc body", "cat > f <<'EOF'\ngit push origin topic\nEOF", nil, false},
		{"commit message", "git commit -m \"$(cat <<'EOF'\nfix: x\n\ngit push origin topic\nEOF\n)\"", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, e := findPushes("/repo", tt.command, 0)
			if (e != nil) != tt.unverifiable || (e != nil && !errors.Is(e, errUnverifiable)) {
				t.Fatalf("err=%v", e)
			}
			if e == nil && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("pushes=%+v want %+v", got, tt.want)
			}
		})
	}
}
func TestHookOutput(t *testing.T) {
	t.Parallel()
	merged := gitstate.ErrMergedBranch.Error() + "\n作業ディレクトリ: /repo"
	for _, tt := range []struct {
		name, command string
		err           error
		wantCall      bool
		reason        string
	}{
		{"unrelated", "echo 'git push'", nil, false, ""},
		{"normal", "git push", nil, true, ""},
		{"unknown", "git push", errors.New("確認不能"), true, ""},
		{"timeout", "git push", context.DeadlineExceeded, true, ""},
		{"canceled", "git push", context.Canceled, true, ""},
		{"same text is not evidence", "git push", errors.New(gitstate.ErrMergedBranch.Error()), true, ""},
		{"confirmed", "git push", gitstate.ErrMergedBranch, true, merged},
		{"wrapped confirmed", "git push", fmt.Errorf("private details: %w", gitstate.ErrMergedBranch), true, merged},
		{"chained", "git add x && git commit -m y && git push origin topic && git rev-parse HEAD", nil, true, ""},
		{"chained confirmed", "git add x && git commit -m y && git push origin topic && git rev-parse HEAD", gitstate.ErrMergedBranch, true, merged},
		{"pipeline and redirect confirmed", "git push origin feature/example 2>&1 | tail -2", gitstate.ErrMergedBranch, true, merged},
		{"dynamic", `git push "$REMOTE"`, nil, false, denyUnverifiable},
		{"unclosed quote", `git push 'origin`, nil, false, ""},
		{"commit message mentioning push", "git commit -m \"$(cat <<'EOF'\nfix: x\n\ngit push origin topic\nEOF\n)\"", nil, false, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			payload, _ := json.Marshal(map[string]any{"cwd": "/repo", "tool_name": "Bash", "tool_input": map[string]string{"command": tt.command}})
			var out bytes.Buffer
			called := false
			e := runForTest(context.Background(), bytes.NewReader(payload), &out, func(context.Context, string, []string) error { called = true; return tt.err })
			if e != nil {
				t.Fatal(e)
			}
			if called != tt.wantCall {
				t.Fatalf("called=%v", called)
			}
			if tt.reason == "" {
				if out.Len() != 0 {
					t.Fatalf("allow output=%s", out.String())
				}
				return
			}
			var v struct {
				Hook map[string]string `json:"hookSpecificOutput"`
			}
			if e = json.Unmarshal(out.Bytes(), &v); e != nil || v.Hook["permissionDecision"] != "deny" || v.Hook["hookEventName"] != "PreToolUse" {
				t.Fatalf("deny output=%s err=%v", out.String(), e)
			}
			if v.Hook["permissionDecisionReason"] != tt.reason {
				t.Fatalf("unexpected reason: %s", out.String())
			}
		})
	}
}
func TestGitCUsesPhysicalDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	other := filepath.Join(root, "other")
	child := filepath.Join(other, "child")
	for _, dir := range []string{work, child} {
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
	}
	link := filepath.Join(work, "link")
	if e := os.Symlink(child, link); e != nil {
		t.Fatal(e)
	}
	var env []string
	for _, v := range os.Environ() {
		key := strings.SplitN(v, "=", 2)[0]
		if !strings.HasPrefix(key, "GIT_") && key != "HOME" && key != "XDG_CONFIG_HOME" {
			env = append(env, v)
		}
	}
	env = append(env, "HOME="+root, "XDG_CONFIG_HOME="+root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		b, e := cmd.Output()
		if e != nil {
			t.Fatal(e)
		}
		return strings.TrimSpace(string(b))
	}
	git(other, "init", "--initial-branch=main")
	for _, command := range []string{"git -C '" + link + "/..' push", "git -C '" + link + "' -C .. push"} {
		pushes, e := findPushes(work, command, 0)
		if e != nil || len(pushes) != 1 {
			t.Fatalf("pushes=%+v err=%v", pushes, e)
		}
		dir := pushes[0].dir
		expected := git(work, "-C", link+"/..", "rev-parse", "--show-toplevel")
		actual := git(dir, "rev-parse", "--show-toplevel")
		if actual != expected {
			t.Fatalf("guard cwd=%s actual git cwd=%s", actual, expected)
		}
	}
}
func TestDenyNamesMergedBranch(t *testing.T) {
	t.Parallel()
	merged := &gitstate.MergedBranchError{Remote: "origin", Ref: "refs/heads/feature/example", PRs: []string{"#12（base example-org/example-repo）"}}
	payload, _ := json.Marshal(map[string]any{"cwd": "/repo", "tool_name": "Bash", "tool_input": map[string]string{"command": "git push"}})
	var out bytes.Buffer
	err := runForTest(context.Background(), bytes.NewReader(payload), &out, func(context.Context, string, []string) error {
		return fmt.Errorf("private details: %w", merged)
	})
	var v struct {
		Hook map[string]string `json:"hookSpecificOutput"`
	}
	if err != nil || json.Unmarshal(out.Bytes(), &v) != nil || v.Hook["permissionDecision"] != "deny" {
		t.Fatalf("err=%v output=%s", err, out.String())
	}
	if want := merged.Error() + "\n作業ディレクトリ: /repo"; v.Hook["permissionDecisionReason"] != want {
		t.Fatalf("reason=%q want %q", v.Hook["permissionDecisionReason"], want)
	}
}

func TestReadFailureIsSilent(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := runForTest(context.Background(), iotest.ErrReader(io.ErrUnexpectedEOF), &out, nil); err != nil || out.Len() != 0 {
		t.Fatalf("err=%v output=%s", err, out.String())
	}
}

func TestUnknownDirectoryIsSilent(t *testing.T) {
	t.Parallel()
	for _, cwd := range []string{"", "relative/path"} {
		payload, _ := json.Marshal(map[string]any{"cwd": cwd, "tool_name": "Bash", "tool_input": map[string]string{"command": "git push"}})
		var out bytes.Buffer
		if err := runForTest(context.Background(), bytes.NewReader(payload), &out, nil); err != nil || out.Len() != 0 {
			t.Fatalf("cwd=%q err=%v output=%s", cwd, err, out.String())
		}
	}
}

func TestExpiredCheckIsSilent(t *testing.T) {
	for _, result := range []error{context.DeadlineExceeded, gitstate.ErrMergedBranch} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), gitstate.PushTimeout)
			defer cancel()
			start := time.Now()
			var out bytes.Buffer
			payload := `{"cwd":"/repo","tool_name":"Bash","tool_input":{"command":"git push"}}`
			err := runForTest(ctx, strings.NewReader(payload), &out, func(ctx context.Context, _ string, _ []string) error {
				<-ctx.Done()
				return result
			})
			if err != nil || out.Len() != 0 || time.Since(start) != gitstate.PushTimeout {
				t.Fatalf("err=%v output=%s elapsed=%v", err, out.String(), time.Since(start))
			}
		})
	}
}

func TestMalformedInput(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"{", "null", "{}", `{"tool_name":"Bash"}`, `{"tool_name":"Bash","tool_input":null}`, `{"tool_name":"Bash","tool_input":{}}`, `{"tool_name":"Bash","tool_input":{"command":null}}`, strings.Repeat("x", (1<<20)+1)} {
		var out bytes.Buffer
		if e := runForTest(context.Background(), strings.NewReader(s), &out, nil); e != nil || out.Len() != 0 {
			t.Fatalf("malformed input must fail open: err=%v output=%s", e, out.String())
		}
	}
}
