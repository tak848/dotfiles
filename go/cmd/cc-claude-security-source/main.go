package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	upstreamURL      = "https://github.com/anthropics/claude-plugins-official.git"
	upstreamRevision = "ea10b213686c90bb1984b2016937310e05336cf3"
	upstreamPath     = "plugins/claude-security"
	stampName        = ".dotfiles-source.json"
)

type stamp struct {
	Key  string `json:"key"`
	Tree string `json:"tree"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "cc-claude-security-source:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return errors.New("引数は受け付けません")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	digest, err := fileHash(executable)
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	// macOS もほかの dotfiles と同じ XDG の置き場所に揃える。
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		cache = xdg
	}
	cache, err = filepath.Abs(filepath.Join(cache, "dotfiles", "claude-security"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	path, err := prepare(ctx, cache, upstreamRevision+"-"+digest, func(ctx context.Context, dir string) (string, error) {
		return fetch(ctx, dir, stderr)
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, path)
	return err
}

// prepare は完成前の内容を公開しない。競合した生成物も同じキーの完成版だけを再利用する。
func prepare(ctx context.Context, cache, key string, fetchSource func(context.Context, string) (string, error)) (string, error) {
	if key == "" || filepath.Base(key) != key || key == "." || key == ".." {
		return "", errors.New("不正なキャッシュキーです")
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return "", err
	}
	final := filepath.Join(cache, key)
	if _, err := os.Lstat(final); err == nil {
		return final, validateCache(final, key)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	// 取得元と加工先を別の新規ディレクトリにし、取得したコードは実行しない。
	source, err := os.MkdirTemp(cache, ".upstream-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(source)
	plugin, err := fetchSource(ctx, source)
	if err != nil {
		return "", fmt.Errorf("公式ソースの取得: %w", err)
	}
	staged, err := os.MkdirTemp(cache, ".generated-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staged)
	if err := copyTree(plugin, staged); err != nil {
		return "", err
	}
	if err := transform(staged); err != nil {
		return "", fmt.Errorf("uv run 対応版の生成: %w", err)
	}
	tree, err := treeHash(staged)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(stamp{Key: key, Tree: tree})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(staged, stampName), append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Rename(staged, final); err != nil {
		if readyErr := validateCache(final, key); readyErr != nil {
			return "", fmt.Errorf("完成版の公開: %w（既存版も使用不可: %v）", err, readyErr)
		}
	}
	return final, validateCache(final, key)
}

func fetch(ctx context.Context, dir string, stderr io.Writer) (string, error) {
	// 親セッションの git 設定・作業ツリー・filter を取得先へ持ち込まない。
	var env []string
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(name, "GIT_") {
			env = append(env, value)
		}
	}
	env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_LFS_SKIP_SMUDGE=1")
	git := func(args ...string) error {
		base := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor="}
		cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
		cmd.Dir, cmd.Env, cmd.Stderr = dir, env, stderr
		return cmd.Run()
	}
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"fetch", "--quiet", "--depth=1", "--filter=blob:none", upstreamURL, upstreamRevision},
		{"sparse-checkout", "set", upstreamPath},
		{"checkout", "--quiet", "--detach", "FETCH_HEAD"},
	} {
		if err := git(args...); err != nil {
			return "", fmt.Errorf("git %s: %w", args[0], err)
		}
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir, cmd.Env, cmd.Stderr = dir, env, stderr
	data, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(data)) != upstreamRevision {
		return "", errors.New("取得した commit が固定 SHA と一致しません")
	}
	return filepath.Join(dir, filepath.FromSlash(upstreamPath)), nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(target, rel), 0o700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("通常ファイル以外は取得できません: %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(target, rel), data, 0o600|(info.Mode().Perm()&0o111))
	})
}

func validateCache(dir, key string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("キャッシュが通常のディレクトリではありません")
	}
	data, err := os.ReadFile(filepath.Join(dir, stampName))
	if err != nil {
		return fmt.Errorf("未完成のキャッシュです: %w", err)
	}
	var got stamp
	if err := json.Unmarshal(data, &got); err != nil {
		return err
	}
	if got.Key != key {
		return errors.New("キャッシュキーが一致しません")
	}
	for _, path := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/hooks.sh", "hooks/hooks.py", "LICENSE", "NOTICE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil || len(data) == 0 {
			return fmt.Errorf("キャッシュの必須ファイルがありません: %s", path)
		}
	}
	digest, err := treeHash(dir)
	if err != nil {
		return err
	}
	if digest != got.Tree {
		return errors.New("加工済みキャッシュの内容が変わっています。既存版は使用しません")
	}
	return nil
}

func treeHash(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("キャッシュ内に通常ファイル以外があります: %s", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == stampName {
			return nil
		}
		digest, err := fileHash(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%o\x00%s\n", filepath.ToSlash(rel), info.Mode().Perm(), digest)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
