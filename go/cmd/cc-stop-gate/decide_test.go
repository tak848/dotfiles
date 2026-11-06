package main

import (
	"strings"
	"testing"
)

// 外から再開される待ちは、背景タスクも cron も無い「作業待機」を 1 回だけ差し戻して確かめさせ、
// Stop hook による継続中（書き直した後）なら通す。背景作業がある場合と、無効なときの差し戻しは今と同じ。
func TestExternalWait(t *testing.T) {
	t.Parallel()
	msg := analyze("任せた作業の結果を待ちます。\n作業待機: 任せた作業の結果")
	for _, mode := range []string{"default", "plan"} {
		otherwise := completionCheck
		if mode == "plan" {
			otherwise = planCheck
		}
		for name, tt := range map[string]struct {
			inflight       int
			externalWait   bool
			stopHookActive bool
			want           decision
		}{
			"inflight":                     {inflight: 1, want: decision{}},
			"inflight while continuing":    {inflight: 1, stopHookActive: true, want: decision{}},
			"inflight external":            {inflight: 1, externalWait: true, want: decision{}},
			"inflight external continuing": {inflight: 1, externalWait: true, stopHookActive: true, want: decision{}},
			"external first":               {externalWait: true, want: block(waitCheck)},
			"external rewritten":           {externalWait: true, stopHookActive: true, want: decision{}},
			"disabled":                     {want: block(noWaitTarget + "\n\n" + otherwise)},
			"disabled while continuing":    {stopHookActive: true, want: block(noWaitTarget + "\n\n" + otherwise)},
		} {
			f := facts{Mode: mode, Message: msg, Inflight: tt.inflight, ExternalWait: tt.externalWait, StopHookActive: tt.stopHookActive}
			if d := decide(f); d != tt.want {
				t.Errorf("%s/%s: got=%v want=%v", mode, name, d, tt.want)
			}
		}
		// 対象が空の「作業待機:」は署名として認めないので、通常の完遂確認になる。
		for _, active := range []bool{false, true} {
			empty := facts{Mode: mode, ExternalWait: true, StopHookActive: active, Message: analyze("作業待機:　")}
			d := decide(empty)
			if d.Decision != "block" || strings.Contains(d.Reason, noWaitTarget) || strings.Contains(d.Reason, waitCheck) {
				t.Fatalf("%s: empty target must block as unsigned: %v", mode, d)
			}
		}
	}
	// 待機以外の署名・署名なしの判定は、外からの待ちの設定にも Stop hook による継続中かにも左右されない。
	for _, mode := range []string{"default", "plan"} {
		for _, m := range []message{{}, {Signature: pledge}, {Signature: pledge, Tells: true}, {Signature: survey, Tells: true}, {Signature: question}} {
			f := facts{Mode: mode, Message: m}
			want := decide(f)
			for _, variant := range []facts{
				{Mode: mode, Message: m, ExternalWait: true},
				{Mode: mode, Message: m, StopHookActive: true},
				{Mode: mode, Message: m, ExternalWait: true, StopHookActive: true},
			} {
				if d := decide(variant); d != want {
					t.Fatalf("%s: a non-waiting decision changed: facts=%+v got=%v want=%v", mode, variant, d, want)
				}
			}
		}
	}
	// 確認済みの Git の問題があっても、待機の判定は Git を見ない（run が照会しない）。
	issues := []string{"未 commit"}
	if d := decide(facts{ExternalWait: true, Message: message{Signature: waiting}, Issues: issues}); d != block(waitCheck) {
		t.Fatalf("external wait check must not carry git issues: %v", d)
	}
	if d := decide(facts{ExternalWait: true, StopHookActive: true, Message: message{Signature: waiting}, Issues: issues}); d != (decision{}) {
		t.Fatalf("rewritten external wait must not be judged by git issues: %v", d)
	}
	// 離脱表現があっても、作業待機の判定は今と同じく離脱表現を見ない。
	tells := analyze("続きは次回\n作業待機: 任せた作業の結果")
	if d := decide(facts{ExternalWait: true, Message: tells}); d != block(waitCheck) {
		t.Fatalf("external wait check must not judge tells: %v", d)
	}
}

// 確認の文は、待つ相手が結果を届けるか・承認が必須か・進められる作業が残っていないかを確かめさせ、
// 書き直すときの署名の形と、質問・最後の text の指示を含む。
func TestWaitCheck(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"【作業待機の前に確かめろ】", "この turn の外から結果を届けてくれるものか", "本当に必須か", "待たずに進められる作業が残っていないか", "「作業待機: <待つ対象>」を書き直せ", askQuestion, finalReport} {
		if !strings.Contains(waitCheck, want) {
			t.Errorf("waitCheck missing %q", want)
		}
	}
	// 同じ turn で別の理由で差し戻された後の作業待機は waitCheck を経ずに通るので、完遂確認でも同じ注意を伝える。
	if !strings.Contains(completionCheck, "「作業待機: <待つ対象>」と書け。人の承認やレビューを待つ前に、それが本当に必須かを確かめろ。") {
		t.Error("completionCheck must warn before waiting for approvals or reviews")
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
