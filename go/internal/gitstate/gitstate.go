// Package gitstate は作業ツリーと ref を変更せず、完了時と push 前の状態を検査する。
// 祖先判定に必要な commit が無い場合は object database だけを fetch で補完する。
// 検査後の変更（TOCTOU）や別ツール経由の push を完全に防止するものではない。
package gitstate

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Runner はシェルを介さずコマンドを実行する。終了コードを持つエラーは ExitCode を保持する。
type Runner func(context.Context, string, string, ...string) (string, error)

// Client はグローバル状態を変更せずコマンド実行をテスト用に差し替える。
type Client struct {
	Runner         Runner
	commandTimeout time.Duration
}

// PushTimeout は push 前の確認全体の予算。巨大リポジトリの交渉処理を
// 短い個別タイムアウトで中断しない。外側の hook はこれより長く設定する。
const PushTimeout = 10 * time.Minute

const splitPush = "push の対象を安全に確認できません。展開や複合処理を分け、リテラルの git push 単独コマンドで再確認してください。"
const maxOutput = 2 << 20

// CheckStop は追加の停止理由を返す。コマンド出力や URL は理由文に含めない。
func CheckStop(ctx context.Context, cwd string, requirePROwners []string) []string {
	return (Client{}).CheckStop(ctx, cwd, requirePROwners)
}

// CheckPush は push サブコマンド以降の引数を検査する。本番の push は実行しない。
func CheckPush(ctx context.Context, cwd string, args []string) error {
	return (Client{}).CheckPush(ctx, cwd, args)
}

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := maxOutput - b.Len()
	if len(p) > left {
		p = p[:left]
		b.overflow = true
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func execute(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GH_PROMPT_DISABLED=1", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	// SSH の独自コマンドを上書きしない。SSH 送信先で独自設定がある場合は destinations が確認不能にする。
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=5")
	}
	var out, stderr limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if out.overflow {
		return "", errors.New("output limit")
	}
	if name == "git" && len(args) > 0 && args[0] == "push" && !stderr.overflow {
		err = classifyPushError(err, out.String(), stderr.String())
	}
	return out.String(), err
}
func (c Client) run(ctx context.Context, dir, name string, args ...string) (string, error) {
	timeout := c.commandTimeout
	if timeout == 0 {
		timeout = 12 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r := c.Runner
	if r == nil {
		r = execute
	}
	out, err := r(ctx, dir, name, args...)
	if len(out) > maxOutput {
		return "", errors.New("output limit")
	}
	return out, err
}
func (c Client) git(ctx context.Context, dir string, args ...string) (string, error) {
	return c.run(ctx, dir, "git", args...)
}

type exitCoder interface {
	error
	ExitCode() int
}

func exitIs(err error, n int) bool {
	e, ok := errors.AsType[exitCoder](err)
	return ok && e.ExitCode() == n
}
func (c Client) config(ctx context.Context, dir, key string) (string, error) {
	s, e := c.git(ctx, dir, "config", "--get", key)
	if exitIs(e, 1) {
		return "", nil
	}
	return strings.TrimSpace(s), e
}
func hasGitDirectory(dir string) bool {
	for {
		if _, e := os.Stat(filepath.Join(dir, ".git")); e == nil || !os.IsNotExist(e) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
func (c Client) repository(ctx context.Context, dir string) (bool, error) {
	s, e := c.git(ctx, dir, "rev-parse", "--is-inside-work-tree")
	if e != nil {
		if exitIs(e, 128) && !hasGitDirectory(dir) {
			return false, nil
		}
		return false, e
	}
	return strings.TrimSpace(s) == "true", nil
}

type destination struct {
	url    string
	repo   string
	remote string
}

var repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// GitHub リポジトリの特定はリテラルの github.com URL に限定する。
// ローカルパス・他ホストの HTTPS は PR 検査の対象外。SSH alias と remote helper は確認不能とする。
func identify(raw string) (string, error) {
	if strings.Contains(raw, "::") {
		return "", errors.New("remote helper")
	}
	var host, path string
	if strings.Contains(raw, "://") {
		u, e := url.Parse(raw)
		if e != nil {
			return "", e
		}
		switch u.Scheme {
		case "https", "http", "ssh", "git", "file":
		default:
			return "", errors.New("transport")
		}
		if u.Scheme == "file" {
			return "", nil
		}
		host = u.Hostname()
		path = u.Path
		if u.Scheme == "ssh" && host != "github.com" {
			return "", errors.New("ssh identity")
		}
		if u.Port() != "" {
			return "", errors.New("port")
		}
	} else if i := strings.Index(raw, ":"); i >= 0 {
		h := raw[:i]
		if j := strings.LastIndex(h, "@"); j >= 0 {
			h = h[j+1:]
		}
		host = h
		path = raw[i+1:]
		if host != "github.com" {
			return "", errors.New("ssh identity")
		}
	} else {
		return "", nil
	}
	if !strings.EqualFold(host, "github.com") {
		return "", nil
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	p := strings.Split(path, "/")
	if len(p) != 2 || !repoPart.MatchString(p[0]) || !repoPart.MatchString(p[1]) {
		return "", errors.New("repository identity")
	}
	return path, nil
}
func (c Client) destinations(ctx context.Context, dir, branch, explicit string) ([]destination, error) {
	s, e := c.git(ctx, dir, "remote")
	if e != nil {
		return nil, e
	}
	remotes := strings.Fields(s)
	selected := explicit
	if selected == "" {
		for _, k := range []string{"branch." + branch + ".pushRemote", "remote.pushDefault", "branch." + branch + ".remote"} {
			selected, e = c.config(ctx, dir, k)
			if e != nil {
				return nil, e
			}
			if selected != "" {
				break
			}
		}
	}
	if selected == "" {
		if len(remotes) == 1 {
			selected = remotes[0]
		} else {
			for _, r := range remotes {
				if r == "origin" {
					selected = r
				}
			}
		}
	}
	if selected == "" {
		if len(remotes) == 0 {
			return nil, nil
		}
		return nil, errors.New("ambiguous remote")
	}
	named := false
	for _, r := range remotes {
		if selected == r {
			named = true
		}
	}
	if named {
		helper, err := c.config(ctx, dir, "remote."+selected+".vcs")
		if err != nil {
			return nil, err
		}
		if helper != "" {
			return nil, errors.New("remote helper")
		}
		s, e = c.git(ctx, dir, "remote", "get-url", "--push", "--all", selected)
	} else if explicit != "" {
		if len(remotes) > 0 {
			// get-url は既存 remote を必要とする。コマンド内だけの空 URL で複数値を解除し、ファイルは変更しない。
			r := remotes[0]
			s, e = c.git(ctx, dir, "-c", "remote."+r+".url=", "-c", "remote."+r+".url="+selected, "-c", "remote."+r+".pushurl=", "remote", "get-url", "--push", "--all", r)
		} else {
			s, e = c.explicitURL(ctx, dir, selected)
		}
	} else {
		return nil, errors.New("unknown configured remote")
	}
	if e != nil {
		return nil, e
	}
	var result []destination
	for _, raw := range strings.Split(strings.TrimSpace(s), "\n") {
		if raw == "" {
			return nil, errors.New("empty URL")
		}
		repo, err := identify(raw)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(raw, "ssh://") || !strings.Contains(raw, "://") && strings.Contains(raw, ":") {
			command, err := c.config(ctx, dir, "core.sshCommand")
			if err != nil {
				return nil, err
			}
			variant := os.Getenv("GIT_SSH_VARIANT")
			if command != "" || os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" || variant != "" && variant != "ssh" {
				return nil, errors.New("custom SSH requires independent verification")
			}
		}
		result = append(result, destination{raw, repo, selected})
	}
	return result, nil
}

// remote が無い場合は get-url が使えない。rewrite の優先順を推測せず、ルールがあれば確認不能とする。
func (c Client) explicitURL(ctx context.Context, dir, raw string) (string, error) {
	s, e := c.git(ctx, dir, "config", "--get-regexp", `^url\..*\.(insteadof|pushinsteadof)$`)
	if e != nil && !exitIs(e, 1) {
		return "", e
	}
	if strings.TrimSpace(s) != "" {
		return "", errors.New("URL rewrite without configured remote")
	}
	return raw, nil
}

// upstreamMismatch は、push.default が simple でブランチ名と upstream 名が異なる設定。
// 照会失敗ではなく設定から決まる状態なので、CheckStop は upstream と比べて判定する。
type upstreamMismatch struct{ mode, merge string }

func (e *upstreamMismatch) Error() string { return "simple name mismatch" }

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// liveLabel は送信先の ref の状態を、差し戻し文用に短く表す。
func liveLabel(live string) string {
	if live == "" {
		return "存在しない"
	}
	return short(live)
}

func prLabel(p pull) string {
	base := ""
	if p.Base.Repo != nil {
		base = p.Base.Repo.FullName
	}
	if p.Number > 0 {
		return fmt.Sprintf("#%d（base %s）", p.Number, base)
	}
	return "（base " + base + "）"
}

// upstreamReason は、upstream の送信先に HEAD が含まれるかで、push すべきものの有無を判定する。
func (c Client) upstreamReason(ctx context.Context, dir string, d destination, branch, head string, m *upstreamMismatch) string {
	live, err := c.live(ctx, dir, d.url, m.merge)
	if err != nil {
		return checkFailure("remote-ref", err, "upstream の送信先 ref を照会できません。")
	}
	if live != "" {
		covered, failure := c.covers(ctx, dir, d.url, head, live)
		if failure != "" {
			return failure
		}
		if covered {
			return ""
		}
	}
	mode := "push.default が " + m.mode
	if m.mode == "" {
		mode = "push.default が未設定（既定の simple）"
	}
	upstream := strings.TrimPrefix(m.merge, "refs/heads/")
	return fmt.Sprintf("ブランチ %s の upstream は %s/%s で、ブランチ名と異なります。%s のため、引数なしの git push では送信先が決まりません。HEAD %s は push 先 %s の %s（%s）に含まれていません。push 先のブランチを明示して push してください。", branch, d.remote, upstream, mode, short(head), d.remote, m.merge, liveLabel(live))
}

// unpushedReason は未反映の内容と、通常の push で済むか（履歴が分岐しているか）を示す。
func (c Client) unpushedReason(ctx context.Context, dir string, d destination, branch, ref, head, live string) string {
	if live == "" {
		return fmt.Sprintf("ブランチ %s の HEAD %s について、push 先 %s に %s がありません。差分を確認して push してください。", branch, short(head), d.remote, ref)
	}
	s := fmt.Sprintf("ブランチ %s の HEAD %s は、push 先 %s の %s（%s）に含まれていません。", branch, short(head), d.remote, ref, short(live))
	ahead, failure := c.covers(ctx, dir, d.url, live, head)
	switch {
	case failure != "":
		return s + "差分を確認して push してください。"
	case ahead:
		return s + "HEAD は送信先より先に進んでいるだけなので、通常の push で反映できます。"
	default:
		return s + "送信先の commit は HEAD の祖先ではなく、履歴が分岐しています（rebase など）。通常の push は拒否されます。自分で履歴を書き換えた結果なら --force-with-lease で push し、送信先に自分以外の commit が含まれる可能性があればユーザーに確認してください。"
	}
}

// stopRef はマージ後の prune で消える remote-tracking ref に依存せず、現在のブランチの送信先 ref を解決する。
func (c Client) stopRef(ctx context.Context, dir, branch, remote string) (string, error) {
	custom, err := c.config(ctx, dir, "remote."+remote+".push")
	if err != nil {
		return "", err
	}
	if custom != "" {
		out, err := c.git(ctx, dir, "push", "--dry-run", "--porcelain", "--no-verify", "--recurse-submodules=no", "--verbose", remote)
		if err != nil {
			return "", err
		}
		updates, err := parsePorcelain(out)
		if err != nil {
			return "", err
		}
		ref := ""
		for _, u := range updates {
			if u.source == "refs/heads/"+branch && !u.deletion {
				if ref != "" && ref != u.target {
					return "", errors.New("multiple push refs")
				}
				ref = u.target
			}
		}
		if !strings.HasPrefix(ref, "refs/heads/") {
			return "", errors.New("current branch not selected")
		}
		return ref, nil
	}
	mode, err := c.config(ctx, dir, "push.default")
	if err != nil {
		return "", err
	}
	tracking, err := c.config(ctx, dir, "branch."+branch+".remote")
	if err != nil {
		return "", err
	}
	merge, err := c.config(ctx, dir, "branch."+branch+".merge")
	if err != nil {
		return "", err
	}
	switch mode {
	case "current", "matching":
		return "refs/heads/" + branch, nil
	case "upstream", "tracking":
		if tracking != remote || !strings.HasPrefix(merge, "refs/heads/") {
			return "", errors.New("missing upstream")
		}
		return merge, nil
	case "", "simple":
		if tracking == remote && merge != "" && merge != "refs/heads/"+branch {
			return "", &upstreamMismatch{mode: mode, merge: merge}
		}
		return "refs/heads/" + branch, nil
	default:
		return "", errors.New("unsupported push default")
	}
}

func (c Client) live(ctx context.Context, dir, remote, ref string) (string, error) {
	s, e := c.git(ctx, dir, "ls-remote", "--exit-code", "--refs", remote, ref)
	if exitIs(e, 2) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) != 1 {
		return "", errors.New("invalid refs")
	}
	v := strings.Fields(lines[0])
	if len(v) != 2 || v[1] != ref || !validSHA(v[0]) {
		return "", errors.New("invalid ref")
	}
	return v[0], nil
}
func validSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

type repoInfo struct {
	FullName      string    `json:"full_name"`
	DefaultBranch string    `json:"default_branch"`
	Fork          bool      `json:"fork"`
	Parent        *repoInfo `json:"parent"`
}
type pull struct {
	Number   int     `json:"number"`
	State    string  `json:"state"`
	MergedAt *string `json:"merged_at"`
	Head     struct {
		Ref  string    `json:"ref"`
		SHA  string    `json:"sha"`
		Repo *repoInfo `json:"repo"`
	} `json:"head"`
	Base struct {
		Repo *repoInfo `json:"repo"`
	} `json:"base"`
}

func (c Client) api(ctx context.Context, dir, endpoint string, out any) error {
	s, e := c.run(ctx, dir, "gh", "api", "--hostname", "github.com", "--method", "GET", endpoint)
	if e != nil {
		return e
	}
	return json.Unmarshal([]byte(s), out)
}
func (c Client) info(ctx context.Context, dir, repo string) (repoInfo, error) {
	var r repoInfo
	e := c.api(ctx, dir, "repos/"+repo, &r)
	if e == nil && (!strings.EqualFold(r.FullName, repo) || r.DefaultBranch == "" || r.Fork && (r.Parent == nil || r.Parent.FullName == "")) {
		e = errors.New("repository metadata")
	}
	return r, e
}
func (c Client) pulls(ctx context.Context, dir string, r repoInfo, branch string) ([]pull, error) {
	candidates := []string{r.FullName}
	if r.Parent != nil && !strings.EqualFold(r.Parent.FullName, r.FullName) {
		candidates = append(candidates, r.Parent.FullName)
	}
	var result []pull
	for _, base := range candidates {
		done := false
		for page := 1; page <= 10; page++ {
			var ps []pull
			endpoint := fmt.Sprintf("repos/%s/pulls?state=all&head=%s&per_page=100&page=%d", base, url.QueryEscape(strings.Split(r.FullName, "/")[0]+":"+branch), page)
			if e := c.api(ctx, dir, endpoint, &ps); e != nil {
				return nil, e
			}
			if ps == nil {
				return nil, errors.New("invalid pull list")
			}
			for _, p := range ps {
				if p.Head.Ref != branch {
					return nil, errors.New("invalid pull ref")
				}
				if p.Head.Repo == nil {
					return nil, errors.New("unknown head repository")
				}
				if !strings.EqualFold(p.Head.Repo.FullName, r.FullName) {
					continue
				}
				if p.Base.Repo == nil || !strings.EqualFold(p.Base.Repo.FullName, base) || !validSHA(p.Head.SHA) || (p.State != "open" && p.State != "closed") {
					return nil, errors.New("invalid pull")
				}
				result = append(result, p)
			}
			if len(ps) < 100 {
				done = true
				break
			}
		}
		if !done {
			return nil, errors.New("pagination bound")
		}
	}
	return result, nil
}
func ownerRequired(repo string, owners []string) bool {
	owner := strings.Split(repo, "/")[0]
	for _, o := range owners {
		if strings.EqualFold(owner, strings.TrimSpace(o)) {
			return true
		}
	}
	return false
}

// CheckStop は検査対象外と取得失敗を区別する。
func (c Client) CheckStop(ctx context.Context, dir string, owners []string) []string {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ok, e := c.repository(ctx, dir)
	if e != nil {
		return []string{checkFailure("worktree", e, "この作業ディレクトリ が読み取り可能な作業ツリーか確認してください。別の作業リポジトリの確認結果では代用できません。")}
	}
	if !ok {
		return nil
	}
	var reasons []string
	status, e := c.git(ctx, dir, "status", "--porcelain=v1", "--untracked-files=normal")
	if e != nil {
		return []string{checkFailure("worktree-status", e, "この作業ディレクトリ で git status を確認してください。変更の有無はまだ判定していません。")}
	}
	if lines := strings.Split(strings.TrimRight(status, "\n"), "\n"); strings.TrimSpace(status) != "" {
		var paths []string
		for _, line := range lines {
			if len(line) > 3 && len(paths) < 2 {
				paths = append(paths, line[3:])
			}
		}
		list := strings.Join(paths, ", ")
		if len(lines) > len(paths) {
			list += fmt.Sprintf(", 他 %d 件", len(lines)-len(paths))
		}
		reasons = append(reasons, fmt.Sprintf("git status で未 commit の変更が %d 件あります（%s）。内容を確認し、必要な変更を commit してください。既存の変更を勝手に破棄・commit せず、判断が必要ならユーザーに確認してください。", len(lines), list))
	}
	branch, e := c.git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if exitIs(e, 1) {
		at := ""
		if h, err := c.git(ctx, dir, "rev-parse", "--verify", "HEAD"); err == nil && validSHA(strings.TrimSpace(h)) {
			at = "（" + short(strings.TrimSpace(h)) + "）"
		}
		return append(reasons, "現在の HEAD"+at+"はブランチを指していません（detached HEAD）。この作業ディレクトリのブランチ状態を確認し、必要な作業を作業ブランチで処理してください。")
	}
	if e != nil {
		return append(reasons, checkFailure("local-branch", e, "この作業ディレクトリのブランチ状態を確認してください。作業用 PR の有無を調べた結果ではありません。"))
	}
	branch = strings.TrimSpace(branch)
	ds, e := c.destinations(ctx, dir, branch, "")
	if e != nil {
		return append(reasons, checkFailure("push-destination", e, "この作業ディレクトリ で pushRemote / remote.pushDefault / remote の URL を確認してください。SSH alias・独自 SSH command・remote helper など、送信先を特定できない設定もこの段階で拒否されます。PR の積み方はこの判定に関係ありません。"))
	}
	if len(ds) == 0 {
		return reasons
	}
	head, e := c.git(ctx, dir, "rev-parse", "--verify", "HEAD")
	head = strings.TrimSpace(head)
	if e != nil || !validSHA(head) {
		return append(reasons, checkFailure("local-head", e, "この作業ディレクトリ の HEAD を確認してください。初回 commit 前かどうかを含め、PR 作成ではなくローカル状態の確認が必要です。"))
	}
	for _, d := range ds {
		ref, err := c.stopRef(ctx, dir, branch, d.remote)
		if m, ok := errors.AsType[*upstreamMismatch](err); ok {
			if r := c.upstreamReason(ctx, dir, d, branch, head, m); r != "" {
				reasons = append(reasons, r)
			}
			continue
		}
		if err != nil {
			reasons = append(reasons, checkFailure("push-ref", err, "push.default / remote の push refspec / tracking 設定を確認してください。PR はまだ照会していないので、base 変更で修復しようとしないでください。"))
			continue
		}
		targetBranch := strings.TrimPrefix(ref, "refs/heads/")
		// main / master は名前だけで既定ブランチ扱いが確定する。
		// PR を要求しないこの経路を、不要な GitHub API の成功に依存させない。
		isDefault := branch == "main" || branch == "master" || targetBranch == "main" || targetBranch == "master"
		var info repoInfo
		if d.repo != "" && !isDefault {
			info, e = c.info(ctx, dir, d.repo)
			if e != nil {
				reasons = append(reasons, checkFailure("github-repository", e, "現在の環境で対象リポジトリの gh api 応答を確認してください。gh auth status の成功だけでは、この API の権限・SSO・応答形式・名前変更を確認したことにはなりません。PR の有無はまだ判定していません。"))
				continue
			}
			isDefault = branch == info.DefaultBranch || targetBranch == info.DefaultBranch
		}
		if d.repo == "" && !isDefault {
			s, err := c.git(ctx, dir, "ls-remote", "--symref", d.url, "HEAD")
			if err != nil {
				reasons = append(reasons, checkFailure("remote-default-branch", err, "送信先の HEAD の照会に失敗しました。送信先への接続を確認してください。"))
				continue
			}
			for _, line := range strings.Split(s, "\n") {
				if line == "ref: refs/heads/"+branch+"\tHEAD" {
					isDefault = true
				}
			}
		}
		live, err := c.live(ctx, dir, d.url, ref)
		if err != nil {
			reasons = append(reasons, checkFailure("remote-ref", err, "送信先 ref の ls-remote 照会に失敗しました。この作業ディレクトリ と実際の push URL に対して確認してください。gh のログイン状態や別リポジトリの照会成功では代用できません。"))
			continue
		}
		covered := false
		if live != "" {
			var failure string
			covered, failure = c.covers(ctx, dir, d.url, head, live)
			if failure != "" {
				reasons = append(reasons, failure)
				continue
			}
		}
		if isDefault {
			if !covered {
				reasons = append(reasons, fmt.Sprintf("ブランチ %s の HEAD %s は、既定ブランチである push 先 %s の %s（%s）に含まれていません。既定ブランチへ直接 push せず、この commit を作業ブランチで扱い、push 先もその作業ブランチにしてください。", branch, short(head), d.remote, ref, liveLabel(live)))
			}
			continue
		}
		var ps []pull
		if d.repo != "" {
			ps, err = c.pulls(ctx, dir, info, targetBranch)
			if err != nil {
				reasons = append(reasons, checkFailure("github-pulls", err, "対象の head repository / branch の PR 一覧を読み取れません。API 権限・応答・ページ上限を確認してください。PR 不在や stacked PR の構造異常という判定ではなく、base の変更や不要な PR 作成を求めていません。"))
				continue
			}
		}
		merged := false
		completed := false
		open := false
		historyFailure := ""
		base := info.FullName
		if info.Parent != nil {
			base = info.Parent.FullName
		}
		// open PR があれば、origin の owner ではなく、その PR の実際の base で判定する。
		bases := map[string]bool{}
		var openPRs, openHeads, mergedPRs []string
		for _, p := range ps {
			if p.MergedAt != nil {
				merged = true
				mergedPRs = append(mergedPRs, prLabel(p))
			}
			if p.State == "open" {
				openPRs = append(openPRs, prLabel(p))
				openHeads = append(openHeads, prLabel(p)+"の head は "+short(p.Head.SHA))
				bases[strings.ToLower(p.Base.Repo.FullName)] = true
				base = p.Base.Repo.FullName
				if p.Head.SHA == head || covered && p.Head.SHA == live {
					open = true
				}
			}
		}
		if len(bases) > 1 {
			reasons = append(reasons, fmt.Sprintf("ブランチ %s を head とする open PR が、複数の base リポジトリにあります（%s）。今回の作業でどの PR を対象とするか、ユーザーに確認してください。", targetBranch, strings.Join(openPRs, "、")))
			continue
		}
		// 現在の open PR で確認できるなら、古い PR の commit を取得する必要はない。
		// 削除済みブランチ、または必要な PR が無い場合にだけ履歴を比較する。
		if live == "" || !open && ownerRequired(base, owners) {
			for _, p := range ps {
				if p.MergedAt == nil {
					continue
				}
				included, failure := c.covers(ctx, dir, d.url, head, p.Head.SHA)
				if included {
					completed = true
					break
				}
				if failure != "" {
					historyFailure = failure
				}
			}
		}
		if live == "" && merged {
			if completed {
				continue
			}
			if historyFailure != "" {
				reasons = append(reasons, historyFailure)
				continue
			}
			reasons = append(reasons, fmt.Sprintf("push 先 %s に %s がありません。このブランチはマージ済み PR %s の head です。HEAD %s は、そのマージ済み PR の head 履歴に含まれていません。削除されたブランチを push で作り直さず、新しい作業ブランチを作り、push 先もそのブランチにしてください。", d.remote, ref, strings.Join(mergedPRs, "、"), short(head)))
			continue
		}
		if !covered {
			reasons = append(reasons, c.unpushedReason(ctx, dir, d, branch, ref, head, live))
		}
		if d.repo != "" && ownerRequired(base, owners) && !open && !completed {
			if len(bases) > 0 {
				if covered {
					reasons = append(reasons, fmt.Sprintf("open PR %s で、現在の HEAD %s（push 先 %s の %s にも反映済み）と一致しません。GitHub 側の反映を待つか、PR の head ブランチを確認してください。PR の作り直しや base の変更は不要です。", strings.Join(openHeads, "、"), short(head), d.remote, ref))
				}
			} else if historyFailure != "" {
				reasons = append(reasons, historyFailure)
			} else {
				searched := info.FullName
				if info.Parent != nil && !strings.EqualFold(info.Parent.FullName, info.FullName) {
					searched += "・" + info.Parent.FullName
				}
				reasons = append(reasons, fmt.Sprintf("%s:%s を head とする PR を %s で探しましたが、HEAD %s を含む PR はありません。draft PR を作成してください。", strings.Split(info.FullName, "/")[0], targetBranch, searched, short(head)))
			}
		}
	}
	return reasons
}

// validatePush は対応するリテラル引数だけを受け付ける。未知のオプションを Git に渡さない。
func validatePush(args []string) (string, error) {
	remote := ""
	positionals := false
	for _, a := range args {
		if strings.ContainsAny(a, "\x00\r\n") {
			return "", errors.New(splitPush)
		}
		if a == "--" {
			positionals = true
			continue
		}
		if !positionals && strings.HasPrefix(a, "-") {
			switch a {
			case "-u", "--set-upstream", "-f", "--force", "--force-with-lease", "--force-if-includes", "--dry-run", "-n", "--porcelain", "--no-verify", "--verify", "--atomic", "--tags", "--all", "--branches", "--follow-tags", "--delete", "-d", "--prune", "--verbose", "-v", "--quiet", "-q", "--recurse-submodules=no":
				continue
			}
			if strings.HasPrefix(a, "--force-with-lease=") {
				continue
			}
			return "", errors.New(splitPush)
		}
		if remote == "" {
			remote = a
		} else if strings.HasPrefix(a, "-") || strings.ContainsAny(a, " *?[\\") {
			return "", errors.New(splitPush)
		}
	}
	return remote, nil
}

type update struct {
	source, target string
	deletion       bool
}

func parsePorcelain(s string) ([]update, error) {
	var result []update
	seenTo, done := false, false
	for _, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		if strings.HasPrefix(line, "To ") {
			seenTo = true
			continue
		}
		if line == "Done" {
			done = true
			continue
		}
		if strings.HasPrefix(line, "Would set upstream of '") && strings.HasSuffix(line, "'") {
			continue
		}
		if line == "" {
			continue
		}
		v := strings.Split(line, "\t")
		if len(v) != 3 || len(v[0]) != 1 || !strings.Contains(" =*+-!", v[0]) {
			return nil, errors.New("porcelain")
		}
		refs := strings.Split(v[1], ":")
		if len(refs) != 2 || !strings.HasPrefix(refs[1], "refs/") || v[0] == "!" {
			return nil, errors.New("porcelain refs")
		}
		result = append(result, update{refs[0], refs[1], v[0] == "-"})
	}
	// verbose な probe の完了行だけなら、送信対象がゼロの正常結果。
	// 更新行がある場合はヘッダも必須。不明な出力は依然として拒否する。
	if !done || len(result) > 0 && !seenTo {
		return nil, errors.New("incomplete porcelain")
	}
	return result, nil
}

func (c Client) CheckPush(ctx context.Context, dir string, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, PushTimeout)
	defer cancel()
	// 値レシーバーのコピーだけを変更し、Stop 側の予算は変えない。
	// 各照会もこの全体予算を共有する。12 秒の制限は push 経路に適用しない。
	c.commandTimeout = PushTimeout
	explicit, e := validatePush(args)
	if e != nil {
		return e
	}
	ok, e := c.repository(ctx, dir)
	if e != nil || !ok {
		return pushFailure("worktree", e, "この作業ディレクトリが Git リポジトリか確認してください。")
	}
	b, e := c.git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if e != nil {
		return pushFailure("local-branch", e, "現在のブランチと detached HEAD の状態を確認してください。")
	}
	branch := strings.TrimSpace(b)
	ds, e := c.destinations(ctx, dir, branch, explicit)
	if e != nil || len(ds) == 0 {
		return pushFailure("push-destination", e, "送信先の URL と push 設定を確認してください。PR の base の問題ではありません。")
	}
	// -- の直前に検査用フラグを置き、--verify があっても検査中の hook を無効にする。本番 argv は変更しない。
	probe := []string{"push"}
	cut := len(args)
	for i, a := range args {
		if a == "--" {
			cut = i
			break
		}
	}
	probe = append(probe, args[:cut]...)
	probe = append(probe, "--dry-run", "--porcelain", "--no-verify", "--recurse-submodules=no", "--verbose")
	probe = append(probe, args[cut:]...)
	out, e := c.git(ctx, dir, probe...)
	if e != nil {
		return pushFailure("push-probe", e, "実送信はしていません。送信元・送信先の状態を確認してください。")
	}
	updates, e := parsePorcelain(out)
	if e != nil {
		return pushFailure("push-output", e, "Git の確認結果を解釈できません。認証失敗や PR 不在と決めつけないでください。")
	}
	for _, d := range ds {
		var info repoInfo
		loaded := false
		for _, u := range updates {
			if u.deletion || !strings.HasPrefix(u.target, "refs/heads/") {
				continue
			}
			live, err := c.live(ctx, dir, d.url, u.target)
			if err != nil {
				return pushFailure("remote-ref", err, "送信先ブランチを照会できません。接続先と Git のアクセス権を確認してください。")
			}
			if live != "" || d.repo == "" {
				continue
			}
			if !loaded {
				info, err = c.info(ctx, dir, d.repo)
				if err != nil {
					return pushFailure("github-repository", err, "対象リポジトリの GitHub API の読み取り権限と応答を確認してください。git push の認証とは別です。")
				}
				loaded = true
			}
			ps, err := c.pulls(ctx, dir, info, strings.TrimPrefix(u.target, "refs/heads/"))
			if err != nil {
				return pushFailure("github-pulls", err, "送信先ブランチのマージ履歴を取得できません。GitHub API の権限と応答を確認してください。PR の積み方を変更する根拠ではありません。")
			}
			for _, p := range ps {
				if p.MergedAt != nil {
					return errors.New("マージ済み PR の削除された head ブランチを再作成する push は拒否しました。新しい作業ブランチを作成してください。")
				}
			}
		}
	}
	return nil
}
