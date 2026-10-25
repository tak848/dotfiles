package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPrepareReuseAndBinaryChange(t *testing.T) {
	source := fixture(t)
	cache := filepath.Join(t.TempDir(), "cache with spaces")
	calls := 0
	fetcher := func(context.Context, string) (string, error) { calls++; return source, nil }
	first, err := prepare(context.Background(), cache, "revision-binary-one", fetcher)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepare(context.Background(), cache, "revision-binary-one", fetcher)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || calls != 1 {
		t.Fatalf("paths = %q / %q, calls = %d", first, second, calls)
	}
	third, err := prepare(context.Background(), cache, "revision-binary-two", fetcher)
	if err != nil {
		t.Fatal(err)
	}
	if third == first || calls != 2 {
		t.Fatal("変換版の変更でキャッシュが更新されません")
	}
	if !filepath.IsAbs(first) {
		t.Fatal("出力が絶対パスではありません")
	}
	if err := validateCache(first, "revision-binary-one"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(source, "hooks/hooks.sh")); got != "#!/bin/sh\nexit 88\n" {
		t.Fatal("元データを書き換えました")
	}
}

func TestPrepareConcurrent(t *testing.T) {
	source, cache := fixture(t), t.TempDir()
	const workers = 4
	var started sync.WaitGroup
	started.Add(workers)
	var calls atomic.Int32
	fetcher := func(context.Context, string) (string, error) {
		calls.Add(1)
		started.Done()
		started.Wait()
		return source, nil
	}
	errs := make(chan error, workers)
	for range workers {
		go func() {
			_, err := prepare(context.Background(), cache, "same-key", fetcher)
			errs <- err
		}()
	}
	for range workers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if err := validateCache(filepath.Join(cache, "same-key"), "same-key"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "same-key" || calls.Load() != workers {
		t.Fatalf("公開された項目が想定外です: %v, calls=%d", entries, calls.Load())
	}
	if _, err := os.Stat(filepath.Join(cache, "same-key", "same-key")); !os.IsNotExist(err) {
		t.Fatal("競合時にディレクトリが入れ子になりました")
	}
}

func TestPrepareFailureAndRetry(t *testing.T) {
	cache, source := t.TempDir(), fixture(t)
	fetcher := func(context.Context, string) (string, error) { return "", errors.New("fixture failure") }
	if _, err := prepare(context.Background(), cache, "key", fetcher); err == nil {
		t.Fatal("取得失敗が成功扱いです")
	}
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 0 {
		t.Fatalf("一時領域が残っています: %v, %v", entries, err)
	}
	fetcher = func(context.Context, string) (string, error) { return source, nil }
	if _, err := prepare(context.Background(), cache, "key", fetcher); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(cache, "key", "hooks/hooks.sh"), "#!/bin/sh\npython3 bad.py\n", 0o700)
	if _, err := prepare(context.Background(), cache, "key", fetcher); err == nil {
		t.Fatal("壊れた完成版を再利用しました")
	}
}

func TestPrepareRejectsUnfinishedAndSymlinks(t *testing.T) {
	for _, tc := range []string{"unfinished", "symlink", "source-symlink", "cancelled", "bad-key", "bad-transform"} {
		t.Run(tc, func(t *testing.T) {
			source, cache := fixture(t), t.TempDir()
			key := "key"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch tc {
			case "unfinished":
				if err := os.Mkdir(filepath.Join(cache, key), 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(source, filepath.Join(cache, key)); err != nil {
					t.Fatal(err)
				}
			case "source-symlink":
				if err := os.Symlink("LICENSE", filepath.Join(source, "link")); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				cancel()
			case "bad-key":
				key = "../escape"
			case "bad-transform":
				put(t, filepath.Join(source, "hooks/hooks.py"), "changed\n", 0o600)
			}
			fetcher := func(context.Context, string) (string, error) { return source, nil }
			if _, err := prepare(ctx, cache, key, fetcher); err == nil {
				t.Fatal("不正な状態を成功扱いしました")
			}
		})
	}
}

func fakeGit(t *testing.T) (string, string) {
	t.Helper()
	dir, source := t.TempDir(), fixture(t)
	log := filepath.Join(dir, "git.log")
	stub := `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$TRACE_FILE"
printf '%s\n' "${GIT_DIR-unset}" >>"$TRACE_FILE.env"
case " $* " in
  *' fetch '*) [ "${FAIL_FETCH-}" != yes ] ;;
  *' checkout '*) mkdir -p "$PWD/plugins"; cp -R "$SOURCE_FIXTURE" "$PWD/plugins/claude-security" ;;
  *' rev-parse '*) printf '%s\n' "${FAKE_REVISION}" ;;
esac
`
	put(t, filepath.Join(dir, "git"), stub, 0o700)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TRACE_FILE", log)
	t.Setenv("SOURCE_FIXTURE", source)
	t.Setenv("FAKE_REVISION", upstreamRevision)
	return dir, log
}

func TestFetchAndRun(t *testing.T) {
	_, log := fakeGit(t)
	t.Setenv("GIT_DIR", "/must-not-be-used")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache with spaces"))
	var stdout, stderr bytes.Buffer
	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v: %s", err, stderr.String())
	}
	path := strings.TrimSuffix(stdout.String(), "\n")
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n") || !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	commands := read(t, log)
	if !strings.Contains(commands, "--depth=1 --filter=blob:none "+upstreamURL+" "+upstreamRevision) || !strings.Contains(commands, "sparse-checkout set "+upstreamPath) {
		t.Fatalf("固定版を取得していません: %s", commands)
	}
	if strings.Contains(read(t, log+".env"), "must-not-be-used") {
		t.Fatal("親の Git 環境が残っています")
	}
	stdout.Reset()
	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if read(t, log) != commands || strings.TrimSpace(stdout.String()) != path {
		t.Fatal("再利用時にネットワークへアクセスしました")
	}
	if err := run([]string{"unexpected"}, io.Discard, io.Discard); err == nil {
		t.Fatal("不明な引数を受け付けました")
	}
}

func TestFetchRejectsOtherRevision(t *testing.T) {
	fakeGit(t)
	t.Setenv("FAKE_REVISION", strings.Repeat("0", 40))
	if _, err := fetch(context.Background(), t.TempDir(), io.Discard); err == nil {
		t.Fatal("異なる SHA を受け入れました")
	}
	t.Setenv("FAIL_FETCH", "yes")
	if _, err := fetch(context.Background(), t.TempDir(), io.Discard); err == nil {
		t.Fatal("fetch 失敗を成功扱いしました")
	}
}

func TestInstallScriptWithFakeClaude(t *testing.T) {
	const (
		install = "plugin install security-uv@tak848-plugins --scope user --yes"
		update  = "plugin update security-uv@tak848-plugins --scope user --yes"
	)
	calls := []string{
		"plugin marketplace add tak848/dotfiles",
		"plugin marketplace update tak848-plugins",
		"plugin install aws-knowledge@tak848-plugins",
		"plugin update aws-knowledge@tak848-plugins",
		"plugin install context7@tak848-plugins",
		"plugin update context7@tak848-plugins",
		install,
		update,
	}
	for _, tt := range []struct {
		name   string
		failAt string // この引数の呼び出しだけ exit 17 にする。空なら全部成功させる
		calls  int    // 失敗した呼び出しまでの件数
	}{
		{"success", "", len(calls)},
		{"install failure", install, len(calls) - 1},
		{"update failure", update, len(calls)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls")
			put(t, filepath.Join(dir, "bin", "claude"), `#!/bin/sh
printf '%s\n' "$*" >>"$TRACE_FILE"
if [ -n "$FAIL_AT" ] && [ "$*" = "$FAIL_AT" ]; then exit 17; fi
`, 0o700)
			text := read(t, filepath.Join("..", "..", "..", "run_onchange_after_50-claude-plugins.sh.tmpl"))
			text = strings.ReplaceAll(text, `{{ template "path-setup.tmpl" . }}`, "")
			script := filepath.Join(dir, "install.sh")
			put(t, script, text, 0o700)
			cmd := exec.Command("/bin/bash", script)
			cmd.Env = []string{"PATH=" + filepath.Join(dir, "bin") + ":/usr/bin:/bin", "HOME=" + dir, "TRACE_FILE=" + log, "FAIL_AT=" + tt.failAt}
			out, err := cmd.CombinedOutput()
			if tt.failAt != "" {
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 17 {
					t.Fatalf("失敗を伝えていません: %v, %s", err, out)
				}
			} else if err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			want := strings.Join(calls[:tt.calls], "\n") + "\n"
			if got := read(t, log); got != want {
				t.Fatalf("CLI calls = %q, want %q", got, want)
			}
		})
	}
}

func TestRepositoryIntegration(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	var market struct {
		Plugins []struct {
			Name           string         `json:"name"`
			DefaultEnabled *bool          `json:"defaultEnabled"`
			Source         jsontext.Value `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(root, ".claude-plugin/marketplace.json"))), &market); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, plugin := range market.Plugins {
		if plugin.Name != "security-uv" {
			continue
		}
		found = true
		var source struct {
			Source  string `json:"source"`
			Command string `json:"command"`
			Mode    string `json:"mode"`
			Timeout int    `json:"timeout"`
		}
		if err := json.Unmarshal(plugin.Source, &source); err != nil {
			t.Fatal(err)
		}
		if plugin.DefaultEnabled == nil || *plugin.DefaultEnabled || source.Source != "command" || source.Mode != "copy" || source.Command != `"$HOME/.claude/bin/cc-claude-security-source"` || source.Timeout != 180 {
			t.Fatalf("catalog が想定外です: %+v", plugin)
		}
	}
	if !found {
		t.Fatal("marketplace にエントリがありません")
	}
	for _, filename := range []string{"run_onchange_after_45-build-statusline.sh.tmpl", "run_onchange_after_50-claude-plugins.sh.tmpl"} {
		text := read(t, filepath.Join(root, filename))
		for _, source := range []string{"main.go", "transform.go"} {
			if !strings.Contains(text, `include "go/cmd/cc-claude-security-source/`+source+`"`) {
				t.Errorf("%s の include に %s がありません", filename, source)
			}
		}
	}
	build := read(t, filepath.Join(root, "run_onchange_after_45-build-statusline.sh.tmpl"))
	if !strings.Contains(build, `-o "$HOME/.claude/bin/cc-claude-security-source" ./go/cmd/cc-claude-security-source`) {
		t.Fatal("生成コマンドの配布がありません")
	}
	install := read(t, filepath.Join(root, "run_onchange_after_50-claude-plugins.sh.tmpl"))
	if !strings.Contains(install, "claude plugin install security-uv@tak848-plugins --scope user --yes") || strings.Contains(install, "plugin enable") || strings.Contains(install, "plugin disable") {
		t.Fatal("導入が無効状態を維持する設計ではありません")
	}
	settings := read(t, filepath.Join(root, "dot_claude/settings.jsonnet"))
	for _, line := range strings.Split(settings, "\n") {
		if strings.Contains(line, "security-uv@") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			t.Fatal("Security の有効化設定を強制しています")
		}
	}
}
