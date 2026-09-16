package gitstate

import (
	"errors"
	"strings"
)

// 既知の Git 拒否だけを固定文言へ変換する。URL、ref 名、stderr 本文は返さない。
// 元の終了コードは Unwrap 経由で保持する。
type gitPushError struct {
	cause string
	err   error
}

func (e *gitPushError) Error() string { return e.cause }
func (e *gitPushError) Unwrap() error { return e.err }

func classifyPushError(err error, stdout, stderr string) error {
	if err == nil {
		return nil
	}
	cause := ""
	switch {
	case strings.Contains(stderr, "src refspec ") && strings.Contains(stderr, " does not match any"):
		cause = "送信元の ref がこの作業ディレクトリに存在しない"
	case strings.Contains(stderr, "has no upstream branch"):
		cause = "現在のブランチに upstream が設定されていない"
	case strings.Contains(stderr, "upstream branch of your current branch does not match"):
		cause = "現在のブランチ名と upstream の名前が一致しない"
	case strings.Contains(stderr, "No configured push destination"):
		cause = "push 送信先が設定されていない"
	case strings.Contains(stdout, "[rejected] (non-fast-forward)") || strings.Contains(stdout, "[rejected] (fetch first)"):
		cause = "送信先にローカルで取り込んでいない更新があり Git が push を拒否した"
	}
	if cause == "" {
		return err
	}
	return &gitPushError{cause: cause, err: err}
}

func pushFailure(stage string, err error, action string) error {
	return errors.New(Feedback(checkFailure(stage, err, action)))
}
