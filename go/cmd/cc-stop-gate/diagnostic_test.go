package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/tak848/dotfiles/go/internal/gitstate"
)

func TestInspectionFailureDoesNotDemandReimplementation(t *testing.T) {
	t.Parallel()
	f := facts{
		CWD:     "/checked/repository",
		Mode:    "default",
		Message: message{Signature: pledge},
		Issues:  []string{gitstate.CheckFailurePrefix + " [github-repository] API を確認できません。"},
	}
	d := decide(f)
	if d.Decision != "block" || !strings.Contains(d.Reason, "必要な情報を取得できませんでした") || !strings.Contains(d.Reason, f.CWD) || !strings.Contains(d.Reason, "GitHub のリポジトリ情報") {
		t.Fatal(d)
	}
	if strings.Contains(d.Reason, completionCheck) || strings.Contains(d.Reason, "【署名の前に全項目") {
		t.Fatal("API failure must not demand repeating the plan checklist")
	}
	if !strings.Contains(d.Reason, "base 変更") || !strings.Contains(d.Reason, "根拠ではない") {
		t.Fatal("must prevent invented PR restructuring")
	}
	if len([]rune(d.Reason)) > 250 {
		t.Fatal("failure-only feedback is too verbose")
	}
	// 修復前に繰り返しても自動解除しない。修復後は通常の完了条件で終了できる。
	if next := decide(f); next != d {
		t.Fatal("failure result drifted")
	}
	f.Issues = nil
	if next := decide(f); next.Decision != "" {
		t.Fatal(next)
	}
}

func TestFailureFeedbackDoesNotTurnIntoScopeChanges(t *testing.T) {
	t.Parallel()
	d := decide(facts{
		Message: message{Signature: survey, Tells: true},
		Issues:  []string{gitstate.CheckFailurePrefix + " [github-pulls] 一覧を確認できません。"},
	})
	if d.Decision != "block" || strings.Contains(d.Reason, completionCheck) || strings.Contains(d.Reason, "【叩き起こし】") {
		t.Fatal(d)
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
	d := decide(facts{
		Message: message{Signature: pledge},
		Issues:  []string{"未 commit の変更があります。", gitstate.CheckFailurePrefix + " [remote-ref] 確認できません。"},
	})
	if d.Decision != "block" || !strings.Contains(d.Reason, "未 commit") || !strings.Contains(d.Reason, "送信先ブランチの commit") || !strings.Contains(d.Reason, completionCheck) {
		t.Fatal(d)
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
		return []string{gitstate.CheckFailurePrefix + " [commit-object] commit を確認できません。"}
	})
	var d decision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if code != 0 || d.Decision != "block" || !strings.Contains(d.Reason, "作業ディレクトリ: /repo/checked") {
		t.Fatal(code, d)
	}
}
