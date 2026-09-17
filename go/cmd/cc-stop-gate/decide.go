package main

import (
	"strings"

	"github.com/tak848/dotfiles/go/internal/gitstate"
	"github.com/tak848/dotfiles/go/internal/render"
)

type facts struct {
	CWD      string
	Mode     string
	Message  message
	Inflight int
	Issues   []string
}

const completionCheck = `【署名の前に全項目を照合しろ】
1. plan があるなら、記憶で済ませず今開き直せ。無ければユーザーの依頼を読み直せ。
2. 依頼された全項目を一つずつ、実装した内容と実際に行った検証結果に照合しろ。
3. やり残しを勝手に「別 PR」「次回」「対象外」にしていないか確認しろ。
一つでも終わっていなければ、その作業を今実行しろ。ユーザーの判断が本当に必要なら AskUserQuestion を使え。
全項目を終えた場合だけ、最終行に装飾なしで次の署名を書け。
完了誓約: <実装・検証を全項目終えた内容の1行要約>
調査完了: <依頼された調査・回答を終えた内容の1行要約>
実行中または予定済みの作業の完了を待つ必要がある場合は「作業待機: <待つ対象>」と書け。`

const planCheck = `【plan mode で説明だけして止まるな】
計画がまとまったなら ExitPlanMode で提示しろ。却下されたなら修正内容をユーザーに説明し、同じターンで再提示しろ。判断が必要なら AskUserQuestion を使え。
調査・質問への回答だけで完了する依頼なら、答え切って最終行に「調査完了: <分かったことの1行要約>」と書け。計画作成を求められているのに、この署名で計画提示を省略するな。
plan mode の計画ファイルや説明用 HTML を commit / push する必要はない。`

func decide(f facts) decision {
	if f.Message.Signature == waiting {
		if f.Inflight > 0 {
			return decision{}
		}
		return block("【作業待機は無効】完了を待つ実行中・予定済みの作業が無く、待つ相手が存在しない。今できる作業を実行しろ。\n\n" + completionCheck)
	}

	var reasons []string
	if f.Mode == "plan" {
		if f.Message.Signature == survey && !f.Message.Tells {
			return decision{}
		}
		if f.Message.Signature == pledge {
			reasons = append(reasons, "【完了誓約は無効】plan mode では実装完了の署名で計画提示を代替できない。")
		}
		if f.Message.Tells {
			reasons = append(reasons, "【叩き起こし】本文が離脱宣言のルールに一致している。計画の途中で止まらず、依頼された範囲をやり切れ。署名があってもこの状態では無効。")
		}
		reasons = append(reasons, planCheck)
		return block(strings.Join(reasons, "\n\n"))
	}

	// 補助照会の失敗は停止理由にも復旧要求にも使わない。
	// 確認できた問題は残し、失敗だけなら通常の完遂・署名確認へ進む。
	var issues []string
	for _, issue := range f.Issues {
		if !gitstate.IsCheckFailure(issue) {
			issues = append(issues, issue)
		}
	}
	if len(issues) > 0 {
		reasons = append(reasons, "以下の Git / PR の状態を確認して対応してください。")
		if f.CWD != "" {
			reasons = append(reasons, "作業ディレクトリ: "+render.Sanitize(f.CWD))
		}
		for _, issue := range issues {
			reasons = append(reasons, safeIssue(gitstate.Feedback(issue)))
		}
		joined := clip(strings.Join(reasons, "\n\n"), 5000)
		return block(joined + "\n\n依頼範囲と既存の変更を確認して対応しろ。他者の変更を勝手に commit・削除するな。ユーザー判断が必要なら AskUserQuestion を使え。\n\n" + completionCheck)
	}

	if f.Message.Signature != unsigned && !f.Message.Tells {
		return decision{}
	}
	if f.Message.Tells {
		reasons = append(reasons, "【叩き起こし】本文が離脱宣言のルールに一致している。続行を宣言するだけで止まったり、残りを勝手に別 PR・次回へ送ったりするな。署名があってもこの状態では無効。文言を消して取り繕わず、実際にやり切れ。")
	}
	reasons = append(reasons, completionCheck)
	return block(strings.Join(reasons, "\n\n"))
}

func safeIssue(text string) string {
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\n') || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, text)
}
