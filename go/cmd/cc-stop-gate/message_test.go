package main

import "testing"

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
			if got := analyze(tt.text); got != tt.want {
				t.Fatalf("analyze() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
