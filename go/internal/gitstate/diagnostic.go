package gitstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CheckFailurePrefix は観測済みの未処理状態と、検査自体の失敗を区別する。
// 外部コマンドの出力や error.Error() は、認証情報を含み得るため連結しない。
const CheckFailurePrefix = "【確認不能】"

func IsCheckFailure(reason string) bool {
	return strings.HasPrefix(reason, CheckFailurePrefix)
}

// Feedback は内部の分類記号を、事実と必要な行動が分かる文章に変換する。
// 判定ロジックは元の理由を使い、表示だけを変える。
func Feedback(reason string) string {
	if !IsCheckFailure(reason) {
		return reason
	}
	rest := strings.TrimSpace(strings.TrimPrefix(reason, CheckFailurePrefix))
	stage, body, ok := strings.Cut(strings.TrimPrefix(rest, "["), "] ")
	if !ok {
		return rest
	}
	labels := map[string]string{
		"worktree":              "Git 作業ツリー",
		"worktree-status":       "ファイル変更の有無",
		"local-branch":          "現在のブランチ",
		"local-head":            "現在の commit",
		"push-destination":      "push 送信先",
		"push-ref":              "push 対象ブランチ",
		"push-probe":            "git push の事前確認",
		"push-output":           "git push の確認結果",
		"github-repository":     "GitHub のリポジトリ情報",
		"remote-default-branch": "送信先の既定ブランチ",
		"remote-ref":            "送信先ブランチの commit",
		"commit-graph":          "commit の祖先関係",
		"fetch-object":          "比較用 commit の取得",
		"github-pulls":          "GitHub の PR 一覧",
		"pr-base":               "PR の対象リポジトリ",
		"pr-head":               "PR と送信先の対応",
		"input-cwd":             "作業ディレクトリ",
		"deadline":              "Git / GitHub への接続",
	}
	if label := labels[stage]; label != "" {
		return label + ": " + body
	}
	return body
}

func checkFailure(stage string, err error, action string) string {
	cause := "応答または設定の検証に失敗"
	var command *exec.Error
	var path *os.PathError
	var exit interface{ ExitCode() int }
	var push *gitPushError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		cause = "照会がタイムアウト"
	case errors.Is(err, context.Canceled):
		cause = "照会がキャンセル"
	case errors.As(err, &command):
		cause = "現在の環境で git または gh を起動できない"
	case errors.As(err, &path):
		cause = "実行先のディレクトリまたはファイルにアクセスできない"
	case errors.As(err, &push):
		cause = push.cause
	case errors.As(err, &exit):
		cause = fmt.Sprintf("照会が終了コード %d で失敗", exit.ExitCode())
	}
	return CheckFailurePrefix + " [" + stage + "] 状態を確認できません（" + cause + "）。" + action
}

// covers は head が other に含まれるかを調べる。比較用 commit が無い場合は
// 解決済みの送信先からその SHA だけを取得し、モデルに差し戻さず再判定する。
func (c Client) covers(ctx context.Context, dir, remote, head, other string) (bool, string) {
	if head == other {
		return true, ""
	}
	_, err := c.git(ctx, dir, "merge-base", "--is-ancestor", head, other)
	if err == nil {
		return true, ""
	}
	if exitIs(err, 1) {
		return false, ""
	}
	if exitIs(err, 128) && ctx.Err() == nil {
		if _, objectErr := c.git(ctx, dir, "cat-file", "-e", other+"^{commit}"); exitIs(objectErr, 128) {
			if remote == "" || !validSHA(other) {
				return false, checkFailure("fetch-object", nil, "取得対象を特定できません。")
			}
			// ref の送り先を指定せず、設定された fetch refspec も使わない。
			// ref / FETCH_HEAD / tags / submodule / maintenance を更新せず
			// object database だけを補完する。認証・出力・時間の制限は共通。
			_, fetchErr := c.git(ctx, dir, "fetch", "--no-write-fetch-head", "--refmap=", "--no-tags", "--recurse-submodules=no", "--no-auto-maintenance", "--no-write-commit-graph", "--", remote, other)
			if fetchErr != nil {
				return false, checkFailure("fetch-object", fetchErr, "比較用 commit の自動取得に失敗しました。")
			}
			// 再帰させない。取得に成功しても判定できなければ一度でエラーを返す。
			_, err = c.git(ctx, dir, "merge-base", "--is-ancestor", head, other)
			if err == nil {
				return true, ""
			}
			if exitIs(err, 1) {
				return false, ""
			}
		}
	}
	return false, checkFailure("commit-graph", err, "commit の祖先関係を判定できません。")
}
