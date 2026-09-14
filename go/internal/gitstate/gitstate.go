// Package gitstate は完了時と push 前の状態を読み取り専用で検査する。
// 検査後の変更（TOCTOU）や別ツール経由の push を完全に防止するものではない。
package gitstate

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
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
type Client struct{ Runner Runner }

const unknown = "Git / GitHub の状態を確認できません。認証・通信・送信先設定を確認してから再実行してください。"
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
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GH_PROMPT_DISABLED=1", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	// SSH の独自コマンドを上書きしない。SSH 送信先で独自設定がある場合は destinations が確認不能にする。
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=5")
	}
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if out.overflow {
		return "", errors.New("output limit")
	}
	return out.String(), err
}
func (c Client) run(ctx context.Context, dir, name string, args ...string) (string, error) {
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
func exitIs(err error, n int) bool {
	var e interface{ ExitCode() int }
	return errors.As(err, &e) && e.ExitCode() == n
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

// stopRef はマージ後の prune で消える remote-tracking ref に依存せず、現在のブランチの送信先 ref を解決する。
func (c Client) stopRef(ctx context.Context, dir, branch, remote string) (string, error) {
	custom, err := c.config(ctx, dir, "remote."+remote+".push")
	if err != nil {
		return "", err
	}
	if custom != "" {
		out, err := c.git(ctx, dir, "push", "--dry-run", "--porcelain", "--no-verify", "--recurse-submodules=no", remote)
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
			return "", errors.New("simple name mismatch")
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
		return []string{unknown}
	}
	if !ok {
		return nil
	}
	var reasons []string
	status, e := c.git(ctx, dir, "status", "--porcelain=v1", "--untracked-files=normal")
	if e != nil {
		return []string{unknown}
	}
	if strings.TrimSpace(status) != "" {
		reasons = append(reasons, "未 commit の変更があります。内容を確認し、必要な変更を commit してください。既存の変更を勝手に破棄・commit せず、判断が必要ならユーザーに確認してください。")
	}
	branch, e := c.git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if e != nil {
		return append(reasons, "ブランチを確定できません。detached HEAD などの状態を確認してください。")
	}
	branch = strings.TrimSpace(branch)
	ds, e := c.destinations(ctx, dir, branch, "")
	if e != nil {
		return append(reasons, unknown)
	}
	if len(ds) == 0 {
		return reasons
	}
	head, e := c.git(ctx, dir, "rev-parse", "--verify", "HEAD")
	head = strings.TrimSpace(head)
	if e != nil || !validSHA(head) {
		return append(reasons, "現在の commit を確認できません。未 commit の作業とブランチ状態を確認してください。")
	}
	for _, d := range ds {
		ref, err := c.stopRef(ctx, dir, branch, d.remote)
		if err != nil {
			reasons = append(reasons, unknown)
			continue
		}
		targetBranch := strings.TrimPrefix(ref, "refs/heads/")
		var info repoInfo
		if d.repo != "" {
			info, e = c.info(ctx, dir, d.repo)
			if e != nil {
				reasons = append(reasons, unknown)
				continue
			}
		}
		isDefault := branch == "main" || branch == "master" || branch == info.DefaultBranch || targetBranch == "main" || targetBranch == "master" || targetBranch == info.DefaultBranch
		if d.repo == "" && !isDefault {
			s, err := c.git(ctx, dir, "ls-remote", "--symref", d.url, "HEAD")
			if err != nil {
				reasons = append(reasons, unknown)
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
			reasons = append(reasons, unknown)
			continue
		}
		if isDefault {
			if live != head { // ローカルが既定ブランチに含まれるなら push は不要。
				if live != "" {
					_, err = c.git(ctx, dir, "merge-base", "--is-ancestor", head, live)
				}
				if live != "" && err != nil && !exitIs(err, 1) {
					reasons = append(reasons, unknown)
				} else if live == "" || exitIs(err, 1) {
					reasons = append(reasons, "既定ブランチ（main / master を含む）に未反映の commit があります。main へ直接 push せず、作業ブランチへ移して確認してください。")
				}
			}
			continue
		}
		covered := live == head
		if live != "" && !covered {
			_, err = c.git(ctx, dir, "merge-base", "--is-ancestor", head, live)
			if err == nil {
				covered = true
			} else if !exitIs(err, 1) {
				reasons = append(reasons, unknown)
				continue
			}
		}
		var ps []pull
		if d.repo != "" {
			ps, err = c.pulls(ctx, dir, info, targetBranch)
			if err != nil {
				reasons = append(reasons, unknown)
				continue
			}
		}
		merged := false
		completed := false
		open := false
		historyUnknown := false
		base := info.FullName
		if info.Parent != nil {
			base = info.Parent.FullName
		}
		// open PR があれば、origin の owner ではなく、その PR の実際の base で判定する。
		bases := map[string]bool{}
		for _, p := range ps {
			if p.MergedAt != nil {
				merged = true
				if p.Head.SHA == head {
					completed = true
				} else {
					_, err = c.git(ctx, dir, "merge-base", "--is-ancestor", head, p.Head.SHA)
					if err == nil {
						completed = true
					} else if !exitIs(err, 1) {
						historyUnknown = true
					}
				}
			}
			if p.State == "open" {
				bases[strings.ToLower(p.Base.Repo.FullName)] = true
				base = p.Base.Repo.FullName
				if p.Head.SHA == head || covered && p.Head.SHA == live {
					open = true
				}
			}
		}
		if len(bases) > 1 {
			reasons = append(reasons, "PR の base が複数あり、対象を確定できません。意図する PR を確認してください。")
			continue
		}
		if live == "" && merged {
			if completed {
				continue
			}
			if historyUnknown {
				reasons = append(reasons, unknown)
				continue
			}
			reasons = append(reasons, "マージ後に削除されたブランチに追加 commit があります。削除ブランチを push で復活させず、新しい作業ブランチで処理してください。")
			continue
		}
		if !covered {
			reasons = append(reasons, "現在の commit が push 送信先に反映されていません。送信先と差分を確認し、push guard の確認を通して push してください。")
		}
		if d.repo != "" && ownerRequired(base, owners) && !open && !completed {
			if len(bases) > 0 {
				if covered {
					reasons = append(reasons, "PR の head と送信先の状態が一致しません。PR が存在しないと決めつけず、状態を再確認してください。")
				}
			} else if historyUnknown {
				reasons = append(reasons, unknown)
			} else {
				reasons = append(reasons, "対象 owner のリポジトリに現在の commit を扱う PR がありません。base を確認して draft PR を作成してください。")
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
	if !seenTo || !done || len(result) == 0 {
		return nil, errors.New("empty porcelain")
	}
	return result, nil
}

func (c Client) CheckPush(ctx context.Context, dir string, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	explicit, e := validatePush(args)
	if e != nil {
		return e
	}
	ok, e := c.repository(ctx, dir)
	if e != nil || !ok {
		return errors.New(unknown)
	}
	b, e := c.git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if e != nil {
		return errors.New(unknown)
	}
	branch := strings.TrimSpace(b)
	ds, e := c.destinations(ctx, dir, branch, explicit)
	if e != nil || len(ds) == 0 {
		return errors.New(unknown)
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
	probe = append(probe, "--dry-run", "--porcelain", "--no-verify", "--recurse-submodules=no")
	probe = append(probe, args[cut:]...)
	out, e := c.git(ctx, dir, probe...)
	if e != nil {
		return errors.New(unknown)
	}
	updates, e := parsePorcelain(out)
	if e != nil {
		return errors.New(unknown)
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
				return errors.New(unknown)
			}
			if live != "" || d.repo == "" {
				continue
			}
			if !loaded {
				info, err = c.info(ctx, dir, d.repo)
				if err != nil {
					return errors.New(unknown)
				}
				loaded = true
			}
			ps, err := c.pulls(ctx, dir, info, strings.TrimPrefix(u.target, "refs/heads/"))
			if err != nil {
				return errors.New(unknown)
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
