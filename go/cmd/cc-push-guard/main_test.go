package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestParsePush(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, command, dir string
		args               []string
		applies, deny      bool
	}{
		{"plain", "git push", "/repo", nil, true, false},
		{"trailing LF", "git push\n", "/repo", nil, true, false},
		{"echo words", "echo git push", "", nil, false, false},
		{"log word", "git log --grep push", "", nil, false, false},
		{"eval words", "eval git push", "", nil, true, true},
		{"if compound", "if true; then git push; fi", "", nil, true, true},
		{"if condition", "if git push; then true; fi", "", nil, true, true},
		{"for compound", "for x in a; do git push; done", "", nil, true, true},
		{"while compound", "while true; do git push; done", "", nil, true, true},
		{"until compound", "until git push; do true; done", "", nil, true, true},
		{"elif compound", "if false; then true; elif git push; then true; fi", "", nil, true, true},
		{"else compound", "if false; then true; else git push; fi", "", nil, true, true},
		{"negation", "! git push", "", nil, true, true},
		{"case compound", "case a in a) git push;; esac", "", nil, true, true},
		{"brace compound", "{ git push; }", "", nil, true, true},
		{"function compound", "function f { git push; }; f", "", nil, true, true},
		{"reserved literal", "echo 'then git push'", "", nil, false, false},
		{"compound literal", "echo 'if true; then git push; fi'", "", nil, false, false},
		{"remote", "git push -u origin HEAD:refs/heads/topic", "/repo", []string{"-u", "origin", "HEAD:refs/heads/topic"}, true, false},
		{"cd quoted", `cd '/work/a b' && git push origin 'topic'`, "/work/a b", []string{"origin", "topic"}, true, false},
		{"cd relative", `cd ../other && git push`, "/other", nil, true, false},
		{"cd end options", `cd -- other && git push`, "/repo/other", nil, true, false},
		{"C escaped", `git -C /work/a\ b push origin topic`, "/work/a b", []string{"origin", "topic"}, true, false},
		{"multiple C", `git -C /work -C other push`, "/work/other", nil, true, false},
		{"quoted dollar", `git -C '/work/$literal' push`, "/work/$literal", nil, true, false},
		{"escaped dollar", `git -C /work/\$literal push`, "/work/$literal", nil, true, false},
		{"dynamic", `git push "$REMOTE"`, "", nil, true, true},
		{"dynamic command", `$GIT push`, "", nil, true, true},
		{"dynamic subcommand", `git $ACTION`, "", nil, true, true},
		{"switch", `git switch topic && git push`, "", nil, true, true},
		{"extra command", `git push; echo done`, "", nil, true, true},
		{"pipe", `git push | tee log`, "", nil, true, true},
		{"eval", `eval 'git push'`, "", nil, true, true},
		{"shell", `bash -c 'git push'`, "", nil, true, true},
		{"subshell", `(git push)`, "", nil, true, true},
		{"substitution", `echo $(git push)`, "", nil, true, true},
		{"quoted substitution", `echo "$(git push)"`, "", nil, true, true},
		{"backtick", "echo `git push`", "", nil, true, true},
		{"environment", `FOO=bar git push`, "", nil, true, true},
		{"wrapper", `command git push`, "", nil, true, true},
		{"redirect", `git push >output`, "", nil, true, true},
		{"git config", `git -c push.default=current push`, "", nil, true, true},
		{"unclosed", `git push 'origin`, "", nil, true, true},
		{"echo", `echo 'git push'`, "", nil, false, false},
		{"literal substitution", `echo '$(git push)'`, "", nil, false, false},
		{"log", `git log --grep='git push'`, "", nil, false, false},
		{"status", `git status`, "", nil, false, false},
		{"comment", `echo ok # git push`, "", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, args, applies, e := parsePush("/repo", tt.command)
			if applies != tt.applies || (e != nil) != tt.deny {
				t.Fatalf("applicable=%v err=%v", applies, e)
			}
			if applies && e == nil && (dir != tt.dir || !reflect.DeepEqual(args, tt.args)) {
				t.Fatalf("dir=%q args=%q", dir, args)
			}
		})
	}
}
func TestHookOutput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		command        string
		err            error
		wantCall, deny bool
	}{{"echo 'git push'", nil, false, false}, {"git push", nil, true, false}, {"git push", errors.New("確認不能"), true, true}, {"git switch topic && git push", nil, false, true}} {
		t.Run(tt.command+strings.TrimSpace(strings.Repeat("x", btoi(tt.deny))), func(t *testing.T) {
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
			if !tt.deny {
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
		})
	}
}
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
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
		dir, _, applies, e := parsePush(work, command)
		if e != nil || !applies {
			t.Fatalf("parse: %v", e)
		}
		expected := git(work, "-C", link+"/..", "rev-parse", "--show-toplevel")
		actual := git(dir, "rev-parse", "--show-toplevel")
		if actual != expected {
			t.Fatalf("guard cwd=%s actual git cwd=%s", actual, expected)
		}
	}
}
func TestMalformedInput(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"{", "null", "{}", `{"tool_name":"Bash"}`, `{"tool_name":"Bash","tool_input":null}`, `{"tool_name":"Bash","tool_input":{}}`, `{"tool_name":"Bash","tool_input":{"command":null}}`, strings.Repeat("x", (1<<20)+1)} {
		var out bytes.Buffer
		if e := runForTest(context.Background(), strings.NewReader(s), &out, nil); e != nil || !strings.Contains(out.String(), `"deny"`) {
			t.Fatalf("malformed input accepted: %v", e)
		}
	}
}
