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

func checkFailure(stage string, err error, action string) string {
	cause := "応答または設定の検証に失敗"
	var command *exec.Error
	var path *os.PathError
	var exit interface{ ExitCode() int }
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		cause = "照会がタイムアウト"
	case errors.Is(err, context.Canceled):
		cause = "照会がキャンセル"
	case errors.As(err, &command):
		cause = "hook の実行環境でコマンドを起動できない"
	case errors.As(err, &path):
		cause = "実行先のディレクトリまたはファイルにアクセスできない"
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
