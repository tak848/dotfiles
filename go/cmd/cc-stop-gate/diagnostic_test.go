package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/tak848/dotfiles/go/internal/gitstate"
)

func TestInspectionFailureDoesNotBlockCompletion(t *testing.T) {
	t.Parallel()
	for _, sig := range []signature{pledge, survey} {
		f := facts{
			CWD:     "/checked/repository",
			Mode:    "default",
			Message: message{Signature: sig},
			Issues:  []string{gitstate.CheckFailurePrefix + " [github-repository] API を確認できません。"},
		}
		if d := decide(f); d != (decision{}) {
			t.Fatal(d)
		}
		// 回数による解除ではなく、初回から同じ判定にする。
		if d := decide(f); d != (decision{}) {
			t.Fatal(d)
		}
	}
}

func TestInspectionFailurePreservesMessageChecks(t *testing.T) {
	t.Parallel()
	for _, m := range []message{{}, {Signature: survey, Tells: true}, {Signature: pledge, Tells: true}} {
		f := facts{Message: m}
		want := decide(f)
		f.Issues = []string{gitstate.CheckFailurePrefix + " [github-pulls] 一覧を確認できません。"}
		if d := decide(f); d.Decision != "block" || d != want {
			t.Fatalf("failure must not change the message check: got=%v want=%v", d, want)
		}
	}
}

func TestFeedbackNeedsNoHookKnowledge(t *testing.T) {
	t.Parallel()
	cases := []facts{
		{Message: message{}},
		{Message: message{Signature: waiting}},
		{Mode: "plan", Message: message{}},
		{Mode: "plan", Message: message{Signature: pledge}},
		{Message: message{Tells: true}},
		{CWD: "/repo", Issues: []string{"未 commit の変更があります。"}},
		{CWD: "/repo", Issues: []string{gitstate.CheckFailurePrefix + " [github-pulls] PR 情報の取得に失敗しました。"}},
	}
	for _, f := range cases {
		d := decide(f)
		if d.Decision != "block" {
			t.Fatal(d)
		}
		for _, forbidden := range []string{"hook", "gate", "補助チェック", "background_tasks", "session_crons", "transcript", "cwd", "検査ID", "検査 ID", "[github-", gitstate.CheckFailurePrefix} {
			if strings.Contains(d.Reason, forbidden) {
				t.Fatalf("implementation detail %q in feedback: %s", forbidden, d.Reason)
			}
		}
	}
}

func TestObservedProblemsRemainBlocking(t *testing.T) {
	t.Parallel()
	for _, problem := range []string{
		"未 commit の変更があります。",
		"送信先に反映されていません。",
		"PR がありません。",
		"現在の HEAD は detached HEAD です。",
		"open PR の base リポジトリが複数あります。",
		"PR は存在しますが、head と送信先の状態が一致しません。",
	} {
		f := facts{CWD: "/repo", Message: message{Signature: pledge}, Issues: []string{problem}}
		want := decide(f)
		for _, issues := range [][]string{
			{problem, gitstate.CheckFailurePrefix + " [remote-ref] 確認できません。"},
			{gitstate.CheckFailurePrefix + " [remote-ref] 確認できません。", problem},
		} {
			f.Issues = issues
			if d := decide(f); d.Decision != "block" || d != want || !strings.Contains(d.Reason, problem) {
				t.Fatalf("confirmed problem lost or failure leaked: %v", d)
			}
		}
	}
}

func TestDiagnosticCWDFromHookInput(t *testing.T) {
	t.Parallel()
	payload := `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/repo/checked","last_assistant_message":"完了誓約: 全項目を確認"}`
	var out bytes.Buffer
	code := run(strings.NewReader(payload), &out, envMap(nil), func(_ context.Context, cwd string, _ []string) []string {
		if cwd != "/repo/checked" {
			t.Fatal(cwd)
		}
		return []string{"未 commit の変更があります。", gitstate.CheckFailurePrefix + " [commit-object] commit を確認できません。"}
	})
	var d decision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if code != 0 || d.Decision != "block" || !strings.Contains(d.Reason, "作業ディレクトリ: /repo/checked") || strings.Contains(d.Reason, "commit を確認できません") {
		t.Fatal(code, d)
	}
}

type branchExitError int

func (e branchExitError) Error() string { return "symbolic-ref failed" }
func (e branchExitError) ExitCode() int { return int(e) }

func TestBranchStateThroughStopHook(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		err     error
		blocked bool
	}{
		{"detached HEAD", branchExitError(1), true},
		{"execution failure", branchExitError(128), false},
		{"timeout", context.DeadlineExceeded, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			branchChecked := false
			client := gitstate.Client{Runner: func(_ context.Context, _ string, name string, args ...string) (string, error) {
				switch name + " " + strings.Join(args, " ") {
				case "git rev-parse --is-inside-work-tree":
					return "true\n", nil
				case "git status --porcelain=v1 --untracked-files=normal":
					return "", nil
				case "git symbolic-ref --quiet --short HEAD":
					branchChecked = true
					return "", tt.err
				case "git rev-parse --verify HEAD":
					return strings.Repeat("a", 40) + "\n", nil
				default:
					t.Fatalf("unexpected command: %s %q", name, args)
					return "", nil
				}
			}}
			payload := `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/repo/checked","last_assistant_message":"完了誓約: 全項目を確認"}`
			var out bytes.Buffer
			if code := run(strings.NewReader(payload), &out, envMap(nil), client.CheckStop); code != 0 || !branchChecked {
				t.Fatalf("code=%d branchChecked=%v", code, branchChecked)
			}
			if !tt.blocked {
				if out.Len() != 0 {
					t.Fatal(out.String())
				}
				return
			}
			var d decision
			if err := json.Unmarshal(out.Bytes(), &d); err != nil || d.Decision != "block" || !strings.Contains(d.Reason, "detached HEAD") {
				t.Fatalf("decision=%v err=%v", d, err)
			}
		})
	}
}

func TestGitStatusTimeoutDoesNotBlockCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		statusCalled := false
		client := gitstate.Client{Runner: func(ctx context.Context, _ string, name string, args ...string) (string, error) {
			switch name + " " + strings.Join(args, " ") {
			case "git rev-parse --is-inside-work-tree":
				return "true\n", nil
			case "git status --porcelain=v1 --untracked-files=normal":
				statusCalled = true
				<-ctx.Done()
				return "", ctx.Err()
			default:
				t.Fatalf("unexpected command: %s %q", name, args)
				return "", nil
			}
		}}
		payload := `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/repo/checked","last_assistant_message":"完了誓約: 全項目を確認"}`
		var out bytes.Buffer
		code := run(strings.NewReader(payload), &out, envMap(nil), client.CheckStop)
		if code != 0 || out.Len() != 0 || !statusCalled || time.Since(start) != 12*time.Second {
			t.Fatalf("code=%d output=%s statusCalled=%v elapsed=%v", code, out.String(), statusCalled, time.Since(start))
		}
	})
}

func TestOverallTimeoutPreservesConfirmedProblems(t *testing.T) {
	for _, problem := range []string{"", "未 commit の変更があります。"} {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			payload := `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/repo/checked","last_assistant_message":"完了誓約: 全項目を確認"}`
			var out bytes.Buffer
			code := run(strings.NewReader(payload), &out, envMap(nil), func(ctx context.Context, _ string, _ []string) []string {
				<-ctx.Done()
				if problem != "" {
					return []string{problem}
				}
				return nil
			})
			if code != 0 || time.Since(start) != checkTimeout {
				t.Fatalf("code=%d elapsed=%v", code, time.Since(start))
			}
			if problem == "" {
				if out.Len() != 0 {
					t.Fatal(out.String())
				}
				return
			}
			var d decision
			if err := json.Unmarshal(out.Bytes(), &d); err != nil || d.Decision != "block" || !strings.Contains(d.Reason, problem) || strings.Contains(d.Reason, "検査時間") {
				t.Fatalf("decision=%v err=%v", d, err)
			}
		})
	}
}
