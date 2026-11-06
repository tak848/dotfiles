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
	// ExternalWait は、ホストが turn の外の出来事を新しい turn として届ける環境か（main.go の externalWaitEnv）。
	ExternalWait bool
	// StopHookActive は、Stop hook による継続中か（Stop 入力の stop_hook_active）。
	// 背景作業の無い作業待機を ExternalWait で通すときだけ使い、ほかの判定には使わない。
	StopHookActive bool
	Issues         []string
}

// askQuestion は、質問の手順を示す 1 文。完遂確認・plan mode・Git の差し戻しで共通に使う。
// tool 呼び出しの前に書いた text はサーバー側で要約されてユーザーに届かないことがある
// （anthropics/claude-code#74558）。ターン最後の text は届くので、判断材料はそこに書かせ、
// AskUserQuestion は Stop の差し戻し（askNow）の後に text 無しで呼ばせる。質問予告は必ず差し戻すので、
// text の質問でターンが終わることは無い。
const askQuestion = "ユーザーの判断が本当に必要なら、質問は必ず AskUserQuestion で行え。判断材料を先に見せる必要があるときは、判断材料を text に書いて最終行を「質問予告: <問いの要約>」にしろ。このゲートが必ず差し戻して AskUserQuestion を呼ばせるので、ターンは終わらない。判断材料が要らない質問なら AskUserQuestion を直接呼べ。"

// finalReport は、照合の結果ではなく報告を最後の text に書かせる指示。完遂確認と resign で共通に使う。
// 呼び出し元のエージェントは最後の text だけを受け取り、tool 呼び出しの前の text は要約されて
// 届かないことがある（askNow と同じ理由）。差し戻しの後に照合の一覧だけを書くと、報告の中身が読み手に届かない。
const finalReport = "照合の過程や一覧は最後の text に書くな。読み手（人、または作業を頼んだエージェント）が受け取るのは最後の text だけのことがある。tool 呼び出しの前の text は要約されて届かないことがあり、この差し戻しより前に書いた報告も届いているとは限らない。最後の text は読み手に渡す報告そのもの（結果・根拠・未確認のこと）にし、署名はその最終行に書け。"

const completionCheck = `【署名の前に全項目を照合しろ】
1. plan があるなら、記憶で済ませず今開き直せ。無ければユーザーの依頼を読み直せ。
2. 依頼された全項目を一つずつ、実装した内容と実際に行った検証結果に照合しろ。
3. やり残しを勝手に「別 PR」「次回」「対象外」にしていないか確認しろ。
一つでも終わっていなければ、その作業を今実行しろ。` + askQuestion + `
` + finalReport + `
全項目を終えた場合だけ、最終行に装飾なしで次の署名を書け。
完了誓約: <実装・検証を全項目終えた内容の1行要約>
調査完了: <依頼された調査・回答を終えた内容の1行要約>
実行中または予定済みの作業の完了を待つ必要がある場合は「作業待機: <待つ対象>」と書け。人の承認やレビューを待つ前に、それが本当に必須かを確かめろ。`

const planCheck = `【plan mode で説明だけして止まるな】
計画がまとまったなら ExitPlanMode で提示しろ。却下されたなら、何をどう変えたかを plan 本文の冒頭に書いて同じターンで再提示しろ。` + askQuestion + `
調査・質問への回答だけで完了する依頼なら、答え切って最終行に「調査完了: <分かったことの1行要約>」と書け。計画作成を求められているのに、この署名で計画提示を省略するな。
plan mode の計画ファイルや説明用 HTML を commit / push する必要はない。`

// askNow は「質問予告」への返答。直前の text はターン最後の text なのでユーザーに表示済み。
// ここで差し戻して、text を含まない AskUserQuestion だけの応答を出させる。
const askNow = `【質問予告を受け付けた】判断材料はユーザーに表示済み。今すぐ AskUserQuestion を呼べ。
AskUserQuestion の前に text を書くな（tool 呼び出しの前の text は要約されてユーザーに届かないことがある）。
question と選択肢は直前に書いた問いに合わせ、判断材料を繰り返すな。`

const noWaitTarget = "「作業待機」と書かれていますが、実行中の背景タスクも予定済みの cron もありません。待つ対象が無いので、今できる作業を実行してください。"

// waitCheck は、背景タスクも cron も無い作業待機を、外から再開される待ちとして通す前に 1 回だけ返す確認。
// 待つ相手が実在するかでは、待つ必要の無い待ち（必須でない承認など）と本物の待ちを見分けられないので、
// 待つ前に確かめさせる。書き直された作業待機は Stop hook による継続中に届くので、そこで通す。
const waitCheck = `【作業待機の前に確かめろ】
1. 待つ相手は、この turn の外から結果を届けてくれるものか（任せた作業の結果、購読した通知、承認を求めた確認の結果など）。結果を届ける仕組みの無いもの（通知を頼んでいない CI、人の返事）を待つと、誰も再開しない。届けてもらう手段があるなら先に使え。
2. 人の承認やレビューを待つなら、それが本当に必須かを確かめたか。決めつけずに、merge を試す・ブランチの保護設定を読むなどで確かめろ。必須でなければ待たずに進めろ。
3. 待たずに進められる作業が残っていないか。残っていれば今実行しろ。
確かめた結果、それでも待つ必要があるときだけ、最終行に同じ形式の署名「作業待機: <待つ対象>」を書き直せ。待つ必要が無くなったら作業を続けろ。` + askQuestion + `
` + finalReport

// resign は、署名の形式を既に使えている場合に完遂確認の全文を繰り返さないための短い指示。
const resign = "対応後、依頼の全項目を照合し直し、同じ形式の署名を使え。" + finalReport

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
	// 質問はユーザーに判断を委ねる正当な中断なので、離脱表現も Git の状態も問わない。
	// 判断材料には「残りは」「次のステップ」のような語が自然に入る。
	if f.Message.Signature == question {
		return block(askNow)
	}
	if f.Message.Signature == waiting {
		if f.Inflight > 0 {
			return decision{}
		}
		// 外から再開される待ちは background_tasks / session_crons に出ないので、
		// 有効なときは待つ対象の書かれた署名（対象が空なら署名として認めていない）を、
		// 1 回確かめさせてから通す。差し戻した後に書き直された作業待機は stop_hook_active が true で届く。
		// 今まで無条件に通していた待ちに確認を足すだけなので、通す条件は前より緩くならない。
		if f.ExternalWait {
			if f.StopHookActive {
				return decision{}
			}
			return block(waitCheck)
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
		joined := clip(strings.Join(reasons, "\n\n"), 5000) + "\n\n依頼範囲と既存の変更を確認して対応しろ。他者の変更を勝手に commit・削除するな。" + askQuestion
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
