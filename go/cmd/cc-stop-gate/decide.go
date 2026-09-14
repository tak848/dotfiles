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
一つでも終わっていなければ署名せず、その作業を今実行しろ。ユーザーの判断が本当に必要なら AskUserQuestion を使え。内部 TODO の完了や git がクリーンであることだけでは、依頼の完遂にはならない。
全項目を終えた場合だけ、最終行に装飾なしで次の署名を書け。
完了誓約: <実装・検証を全項目終えた内容の1行要約>
調査完了: <依頼された調査・回答を終えた内容の1行要約>
背景作業の完了を待つ必要がある場合は「作業待機: <待つ対象>」と書け。ただし hook が稼働中の仕事の実在を確認する。署名は自己確認であり、hook が意味的な完遂を保証するものではない。`

const planCheck = `【plan mode で説明だけして止まるな】
計画がまとまったなら ExitPlanMode で提示しろ。却下されたなら修正内容をユーザーに説明し、同じターンで再提示しろ。判断が必要なら AskUserQuestion を使え。
調査・質問への回答だけで完了する依頼なら、答え切って最終行に「調査完了: <分かったことの1行要約>」と書け。計画作成を求められているのに、この署名で計画提示を省略するな。
plan mode の計画ファイルや説明用 HTML に commit / push / PR 作成は要求しない。`

func decide(f facts) decision {
	if f.Message.Signature == waiting {
		if f.Inflight > 0 {
			return decision{}
		}
		return block("【作業待機は無効】background_tasks も session_crons も空で、待つ相手が存在しない。自己申告だけで待機にはできない。今できる作業を続けろ。\n\n" + completionCheck)
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

	if len(f.Issues) > 0 {
		onlyFailures := true
		for _, issue := range f.Issues {
			if !gitstate.IsCheckFailure(issue) {
				onlyFailures = false
			}
		}
		if onlyFailures {
			reasons = append(reasons, "【補助チェックの実行失敗】")
		} else {
			reasons = append(reasons, "【補助チェック未完了】観測済みの未処理状態と、【確認不能】の検査失敗を区別しろ。署名だけで状態を覆すことはできない。")
		}
		if f.CWD != "" {
			reasons = append(reasons, "検査対象 cwd: "+render.Sanitize(f.CWD))
		}
		for _, issue := range f.Issues {
			reasons = append(reasons, safeIssue(issue))
		}
		joined := clip(strings.Join(reasons, "\n\n"), 5000)
		if onlyFailures {
			// 同じ不明理由で plan を照合し直させると、モデルが架空の原因を
			// 作って PR の構造や依頼範囲を変えてしまう。検査の復旧だけに絞る。
			return block(joined + "\n\n検査エラーは PR の base 変更や依頼のやり直しを求める根拠ではない。必要な判断だけ AskUserQuestion で確認しろ。")
		}
		return block(joined + "\n\n依頼範囲と既存の変更を確認して片付けろ。全変更を無条件に commit したり、他者の変更を削除して通過したりするな。確認不能を根拠に PR の積み方を変えるな。ユーザー判断が必要なら AskUserQuestion を使え。hook や設定を勝手に無効化するな。\n\n" + completionCheck)
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
