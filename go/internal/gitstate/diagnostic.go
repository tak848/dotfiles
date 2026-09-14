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

// covers は head が other に含まれるかを調べる。未取得の commit を
// 「未 push」や「認証異常」と誤って診断しない。
func (c Client) covers(ctx context.Context, dir, head, other string) (bool, string) {
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
			return false, CheckFailurePrefix + " [commit-object] 比較対象の commit " + other + " をローカルで確認できません。ls-remote の SHA は取得できても、commit 本体が未 fetch の場合は祖先関係を判定できません。hook 入力の cwd のリポジトリで対象 ref を fetch してから再確認してください。これは未 push・PR 不在・plan 未完了という判定ではありません。"
		}
	}
	return false, checkFailure("commit-graph", err, "hook 入力の cwd で比較対象の commit と祖先関係を確認してください。PR の base を変える理由にはなりません。")
}
