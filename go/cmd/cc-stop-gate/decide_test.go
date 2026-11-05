package main

import (
	"strings"
	"testing"
)

// 外から再開される待ちは、背景タスクも cron も無いまま「作業待機」で通す。
// 無効なときの差し戻しは今と同じ。
func TestExternalWait(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"default", "plan"} {
		waiting := facts{Mode: mode, Message: analyze("任せた作業の結果を待ちます。\n作業待機: 任せた作業の結果")}
		if d := decide(waiting); d.Decision != "block" || !strings.Contains(d.Reason, noWaitTarget) {
			t.Fatalf("%s: disabled external wait must block: %v", mode, d)
		}
		waiting.ExternalWait = true
		if d := decide(waiting); d != (decision{}) {
			t.Fatalf("%s: external wait must pass: %v", mode, d)
		}
		// 対象が空の「作業待機:」は署名として認めないので、通常の完遂確認になる。
		empty := facts{Mode: mode, ExternalWait: true, Message: analyze("作業待機:　")}
		d := decide(empty)
		if d.Decision != "block" || strings.Contains(d.Reason, noWaitTarget) {
			t.Fatalf("%s: empty target must block as unsigned: %v", mode, d)
		}
	}
	// 待機以外の署名・署名なしの判定は変えない。
	for _, m := range []message{{}, {Signature: pledge, Tells: true}, {Signature: question}} {
		f := facts{Message: m}
		want := decide(f)
		f.ExternalWait = true
		if d := decide(f); d != want {
			t.Fatalf("external wait changed a non-waiting decision: got=%v want=%v", d, want)
		}
	}
	// 確認済みの Git の問題があっても、待機の判定は Git を見ない（run が照会しない）。
	if d := decide(facts{ExternalWait: true, Message: message{Signature: waiting}, Issues: []string{"未 commit"}}); d != (decision{}) {
		t.Fatalf("external wait must not be judged by git issues: %v", d)
	}
}

// 差し戻しを受けたモデルが最後の text を照合の一覧にしないよう、完遂確認と resign の両方で
// 最後の text を読み手への報告そのものにさせる。
func TestFinalTextIsTheReport(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{"completionCheck": completionCheck, "resign": resign} {
		for _, want := range []string{"照合の過程や一覧は最後の text に書くな", "読み手に渡す報告そのもの", "署名はその最終行に書け", "作業を頼んだエージェント"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
	unsigned := decide(facts{Message: message{}})
	if !strings.Contains(unsigned.Reason, finalReport) {
		t.Fatalf("completion check must carry the final report instruction: %s", unsigned.Reason)
	}
	signed := decide(facts{CWD: "/repo", Message: message{Signature: pledge}, Issues: []string{"未 commit の変更があります。"}})
	if !strings.HasSuffix(signed.Reason, resign) {
		t.Fatalf("resign must carry the final report instruction: %s", signed.Reason)
	}
	// 質問予告への返答は AskUserQuestion だけを求めるので、報告の指示を混ぜない。
	if d := decide(facts{Message: message{Signature: question}}); strings.Contains(d.Reason, finalReport) {
		t.Fatalf("askNow must stay a bare AskUserQuestion instruction: %s", d.Reason)
	}
}
