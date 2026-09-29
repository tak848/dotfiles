package main

import (
	"strings"
	"testing"
)

func TestTellQuotesMatchedPhrase(t *testing.T) {
	t.Parallel()
	m := analyze("修正しました。残りは別 PR で対応します。\n完了誓約: 完了")
	if m.Tell != "残りは" {
		t.Fatalf("Tell = %q", m.Tell)
	}
	d := decide(facts{Message: m})
	if !strings.Contains(d.Reason, "「残りは」（修正しました。残りは別 PR で対応します。）") || !strings.Contains(d.Reason, "残作業が無い") {
		t.Fatal(d.Reason)
	}
	long := analyze(strings.Repeat("あ", 200) + "続けます" + strings.Repeat("い", 200))
	if long.Excerpt != "…"+strings.Repeat("あ", excerptRunes)+"続けます"+strings.Repeat("い", excerptRunes)+"…" {
		t.Fatalf("excerpt = %q", long.Excerpt)
	}
	multi := analyze("前の行\n続けます\n次の行")
	if multi.Excerpt != "続けます" {
		t.Fatalf("excerpt must stay on the matched line: %q", multi.Excerpt)
	}
}

// 質問予告への返答は AskUserQuestion の呼び出しだけを求める。完遂確認や離脱表現の差し戻しを
// 混ぜると、モデルが判断材料を書き直して再び tool 呼び出し前の text に置いてしまう。
func TestQuestionAsksOnly(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"default", "plan"} {
		d := decide(facts{Mode: mode, Message: analyze("残りは B 案の判断だけです。\n質問予告: B 案にするか"), Issues: []string{"未 commit"}})
		if d.Decision != "block" || d.Reason != askNow {
			t.Fatalf("%s: reason = %q", mode, d.Reason)
		}
	}
	for _, text := range []string{completionCheck, planCheck} {
		if !strings.Contains(text, "質問予告: <問いの要約>") {
			t.Fatalf("question procedure missing: %s", text)
		}
	}
}

func TestAnalyze(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		text string
		want message
	}{
		"empty":                           {"", message{}},
		"inline code is checked":          {"`next step`\n調査完了: 例を確認", message{Signature: survey, Tells: true}},
		"indented code is checked":        {"    next step\n調査完了: 例を確認", message{Signature: survey, Tells: true}},
		"pledge":                          {"実装と検証を完了しました。\n完了誓約: 全項目完了\n\n", message{Signature: pledge}},
		"survey fullwidth":                {"調査完了：仕様を確認", message{Signature: survey}},
		"waiting":                         {"作業待機: agent の完了", message{Signature: waiting}},
		"question":                        {"| 案 | 影響 |\n|---|---|\n質問予告: どちらの案にするか", message{Signature: question}},
		"question fullwidth":              {"質問予告：どちらの案にするか", message{Signature: question}},
		"question empty summary":          {"質問予告: ", message{}},
		"question tells still analyzed":   {"残りは判断だけ\n質問予告: 方針", message{Signature: question, Tells: true}},
		"CRLF":                            {"確認しました。\r\n完了誓約: 全項目完了\r\n", message{Signature: pledge}},
		"empty summary":                   {"完了誓約: \t", message{}},
		"not final":                       {"完了誓約: 完了\n追加の説明", message{}},
		"decorated":                       {"**完了誓約: 完了**", message{}},
		"indented":                        {" 完了誓約: 完了", message{}},
		"tells":                           {"続けます。", message{Tells: true}},
		"signed tells":                    {"残りは別 PR で対応します。\n完了誓約: 完了", message{Signature: pledge, Tells: true}},
		"english":                         {"I WILL CONTINUE\n調査完了: 確認", message{Signature: survey, Tells: true}},
		"JA embedded lower case":          {"今回は mvp です。", message{Tells: true}},
		"signature excluded":              {"完了誓約: 残タスクなしを確認", message{Signature: pledge}},
		"quoted":                          {"> 続けます\n  > next step\n調査完了: 引用を確認", message{Signature: survey}},
		"fenced":                          {"```text\n続けます\n```\n調査完了: 例を確認", message{Signature: survey}},
		"tilde fence":                     {"~~~\nnext step\n~~~\n調査完了: 例を確認", message{Signature: survey}},
		"indented fence":                  {"  ```\n続けます\n  ```\n調査完了: 例を確認", message{Signature: survey}},
		"shorter inner fence":             {"````\n```\n続けます\n````\n調査完了: 例を確認", message{Signature: survey}},
		"mismatched fence":                {"```\n~~~\n続けます\n```\n調査完了: 例を確認", message{Signature: survey}},
		"quoted fake fence":               {"> ```\n続けます\n調査完了: 確認", message{Signature: survey, Tells: true}},
		"signature inside unclosed fence": {"```\n完了誓約: 完了", message{}},
		"quoted signature":                {"> 完了誓約: 完了", message{}},
		"negation still matches":          {"未実装箇所はありません。\n完了誓約: 完了", message{Signature: pledge, Tells: true}},
		"ordinary complete":               {"全項目を確認しました。\n完了誓約: 実装・検証完了", message{Signature: pledge}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := analyze(tt.text)
			if got.Signature != tt.want.Signature || got.Tells != tt.want.Tells || (got.Tell != "") != got.Tells {
				t.Fatalf("analyze() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
