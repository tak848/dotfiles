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

const noWaitTarget = "「作業待機」と書かれていますが、実行中の背景タスクも予定済みの cron もありません。待つ対象が無いので、今できる作業を実行してください。"

// resign は、署名の形式を既に使えている場合に完遂確認の全文を繰り返さないための 1 文。
const resign = "対応後、依頼の全項目を照合し直し、最終行に同じ形式の署名を書け。"

// tellReason は、本文のどこが一致したかを引用する。否定文でも一致し得るため、
// 残作業がある場合と無い場合の行動を両方示す。
func tellReason(m message, plan bool) string {
	s := "本文の「" + render.Sanitize(m.Tell) + "」（" + render.Sanitize(m.Excerpt) + "）が、途中で止める・先送りする表現に一致しました。署名があっても、このままでは終了できません。\n"
	if plan {
		return s + "計画がまとまっているなら提示してください。一致が否定文などによる誤りなら、その表現を使わずに書き直してください。"
	}
	return s + "実際に残っている作業があれば、今実行してください。残作業が無い（否定文などで一致した）なら、その表現を使わずに事実を書き、署名し直してください。"
}

func decide(f facts) decision {
	plan := f.Mode == "plan"
	if f.Message.Signature == waiting {
		if f.Inflight > 0 {
			return decision{}
		}
		if plan {
			return block(noWaitTarget + "\n\n" + planCheck)
		}
		return block(noWaitTarget + "\n\n" + completionCheck)
	}

	var reasons []string
	if plan {
		if f.Message.Signature == survey && !f.Message.Tells {
			return decision{}
		}
		if f.Message.Signature == pledge {
			reasons = append(reasons, "【完了誓約は無効】plan mode では実装完了の署名で計画提示を代替できない。")
		}
		if f.Message.Tells {
			reasons = append(reasons, tellReason(f.Message, true))
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
		joined := clip(strings.Join(reasons, "\n\n"), 5000) + "\n\n依頼範囲と既存の変更を確認して対応しろ。他者の変更を勝手に commit・削除するな。ユーザー判断が必要なら AskUserQuestion を使え。"
		if f.Message.Tells {
			return block(joined + "\n\n" + tellReason(f.Message, false) + "\n\n" + completionCheck)
		}
		if f.Message.Signature == unsigned {
			return block(joined + "\n\n" + completionCheck)
		}
		return block(joined + "\n\n" + resign)
	}

	if f.Message.Signature != unsigned && !f.Message.Tells {
		return decision{}
	}
	if f.Message.Tells {
		reasons = append(reasons, tellReason(f.Message, false))
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
