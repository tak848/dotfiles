package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildScript は chezmoi apply 時に Go バイナリをビルドするスクリプト。
// 冒頭の include リストのハッシュが変わったときだけ再実行されるため、
// リストから漏れたファイルは編集しても再ビルドされず、古いバイナリが
// 残り続ける。エラーも警告も出ないので、ここで網羅を検証する。
const buildScript = "run_onchange_after_45-build-statusline.sh.tmpl"

var (
	includeRe = regexp.MustCompile(`include\s+"([^"]+)"`)
	buildRe   = regexp.MustCompile(`go build\s[^\n]*\./go/cmd/([\w-]+)`)
)

// repoRoot はこのテストファイルから見たリポジトリルートを返す。
func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	return root
}

func TestBuildScriptCoversSources(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, buildScript))
	if err != nil {
		// ソースツリーの外から実行された場合は検証しようがない。
		t.Skipf("%s not found: %v", buildScript, err)
	}
	script := string(data)

	included := map[string]bool{}
	for _, m := range includeRe.FindAllStringSubmatch(script, -1) {
		included[m[1]] = true
	}
	if len(included) == 0 {
		t.Fatalf("%s has no include directives", buildScript)
	}

	want := []string{"go.mod", "go.sum"}

	// ビルドしているコマンドのソースは必ず追跡対象にする。ビルドしていない
	// go/cmd 配下のディレクトリ（過去の名残など）は対象外でよい。
	for _, m := range buildRe.FindAllStringSubmatch(script, -1) {
		want = append(want, goFilesUnder(t, root, filepath.Join("go", "cmd", m[1]))...)
	}
	// 共有パッケージはどのバイナリから使われるか追いにくいので一律で入れる。
	want = append(want, goFilesUnder(t, root, filepath.Join("go", "internal"))...)

	for _, path := range want {
		if !included[path] {
			t.Errorf("%s is missing `include \"%s\"`; edits to it would not trigger a rebuild", buildScript, path)
		}
	}
}

// TestBuildScriptBuildsStatuslines は statusline 系のビルド行が消えていない
// ことを確かめる。settings.jsonnet がバイナリを指しているので、ビルド行が
// 無いと statusline が丸ごと出なくなる。
func TestBuildScriptBuildsStatuslines(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, buildScript))
	if err != nil {
		t.Skipf("%s not found: %v", buildScript, err)
	}

	built := map[string]bool{}
	for _, m := range buildRe.FindAllStringSubmatch(string(data), -1) {
		built[m[1]] = true
	}
	for _, name := range []string{"cc-statusline", "cc-subagent-statusline"} {
		if !built[name] {
			t.Errorf("%s does not build ./go/cmd/%s", buildScript, name)
		}
	}
}

// goFilesUnder は dir 以下の非テスト Go ファイルを、リポジトリルートからの
// スラッシュ区切りパスで返す。
func goFilesUnder(t *testing.T, root, dir string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return files
}
