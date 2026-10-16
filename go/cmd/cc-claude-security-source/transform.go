package main

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const runner = "uv run --no-project --no-config --python 3.13 python3"

// 上流の launcher は Python の探索も直呼びするため、uv だけを使う入口に置き換える。
const hookLauncher = `#!/bin/sh
set -eu
case "${1-}" in
  tip)
    if [ "${GITHUB_ACTIONS-}" = true ] || [ "${CI-}" = true ]; then exit 0; fi
    case "${CLAUDE_SECURITY_SCAN_TIP-}" in [Oo][Ff][Ff]) exit 0 ;; esac
    unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR
    export GIT_TERMINAL_PROMPT=0 GIT_NO_LAZY_FETCH=1
    set -- tip "${2-}" "${CLAUDE_PLUGIN_DATA-}" "${CLAUDE_SECURITY_SCAN_TIP-}"
    ;;
  banner) set -- banner "${CLAUDE_PLUGIN_DATA-}" ;;
  metrics|unanswered) ;;
  *) printf '%s\n' 'Claude Security: 不明な hook です。' >&2; exit 2 ;;
esac
exec ` + runner + ` "$(dirname -- "$0")/hooks.py" "$@"
`

const pythonPolicy = `

## Python の実行方法（dotfiles のローカル版）

同梱の Python helper は必ず ` + "`" + runner + "`" + ` に絶対パスと引数を続けて実行する。出力や workflow が helper の名前だけを示した場合も、この呼び出し方を使う。Python インタープリターの直呼びや uvx への fallback はしない。uv が使えない・失敗する場合は停止して報告する。

検証対象の Python テストも必ず uv run 経由（例: uv run pytest、uv run python3 -m pytest）にする。テストには対象プロジェクトの依存を使うため、helper 用の --no-project を付けない。make や shell を使って直呼び禁止を迂回せず、実行方法が不明なら勝手に直接起動しない。
`

type replacement struct {
	path, old, new string
	count          int
}

const localInstallation = `## Installation

これは dotfiles で取得・加工するローカル版です。通常は tak848/dotfiles の chezmoi update で、生成コマンドの配布とプラグインの導入をまとめて行います。公式版とは二重導入しません。

生成コマンドが配布済みの環境で手動導入する場合は、自前マーケットプレイスを使います。

    claude plugin marketplace add tak848/dotfiles
    claude plugin install security-uv@tak848-plugins --scope user

初回は無効です。使うときだけ /plugin の Installed から security-uv を明示的に有効化してください。必要なら /reload-plugins で反映します。有効化後の完全な skill 名は /security-uv:claude-security です。

同梱の Python helper と hook は uv run 経由で動き、直呼びにはフォールバックしません。
`

func patchReadme(dir string) error {
	path := filepath.Join(dir, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(data)
	start, end := "## Installation\n", "\n## Getting started\n"
	if strings.Count(text, start) != 1 || strings.Count(text, end) != 1 {
		return errors.New("README の Installation 節が想定外です")
	}
	begin, finish := strings.Index(text, start), strings.Index(text, end)
	if finish <= begin {
		return errors.New("README の節の順序が想定外です")
	}
	text = text[:begin] + strings.TrimRight(localInstallation, "\n") + "\n" + text[finish:]
	return os.WriteFile(path, []byte(text), 0o600)
}

func transform(dir string) error {
	if err := patchReadme(dir); err != nil {
		return err
	}
	// この固定版の入口を明示する。件数が変わった版を黙って部分変換しない。
	edits := []replacement{
		{"skills/claude-security/SKILL.md", `python3 "`, runner + ` "`, 4},
		{"skills/claude-security/jobs/scan-codebase.md", `python3 "`, runner + ` "`, 4},
		{"skills/claude-security/jobs/scan-changes.md", `python3 "`, runner + ` "`, 4},
		{"skills/claude-security/jobs/suggest-patches.md", `python3 "`, runner + ` "`, 5},
		{"skills/claude-security/specs/patch-spec.md", "python -m pytest", "uv run python3 -m pytest", 1},
		{"scripts/save_result.py", "run render_report.py", "run " + runner + ` \"SCRIPTS/render_report.py\"`, 1},
		{"scripts/save_result.py", "run save_result.py", "run " + runner + ` \"SCRIPTS/save_result.py\"`, 1},
		{"workflows/scan.js", "Run save_result.py", "Run " + runner + ` \"SCRIPTS/save_result.py\"`, 2},
		{"workflows/scan.js", "run write_scan_meta.py", "run " + runner + ` \"SCRIPTS/write_scan_meta.py\"`, 1},
	}
	prefix, err := json.Marshal(strings.Fields(runner))
	if err != nil {
		return err
	}
	anchor := "    if words[:2] == [PY_LAUNCHER, PY_VERSION_OPTION]:"
	// interpreter の単語は従来の parser に残す。裸の python3 には一致させない。
	unwrap := "    uv_prefix = " + string(prefix) + "\n" +
		"    if words[:len(uv_prefix)] != uv_prefix:\n" +
		"        return None\n" +
		"    words = words[len(uv_prefix) - 1:]\n" + anchor
	edits = append(edits, replacement{"hooks/hooks.py", anchor, unwrap, 1})
	for _, edit := range edits {
		if err := replace(dir, edit); err != nil {
			return err
		}
	}
	for _, path := range []string{
		"hooks/hooks.py", "scripts/render_report.py", "scripts/save_result.py",
		"scripts/write_scan_meta.py", "scripts/patch_artifacts.py",
	} {
		if err := replace(dir, replacement{path, "#!/usr/bin/env python3\n", "#!/usr/bin/env -S " + runner + "\n", 1}); err != nil {
			return err
		}
	}
	for _, path := range []string{
		"skills/claude-security/role.md", "agents/patch-generator.md", "agents/patch-verifier.md",
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		data, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(strings.TrimRight(string(data), "\n")+pythonPolicy), 0o600); err != nil {
			return err
		}
	}
	if err := patchJSON(dir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks", "hooks.sh"), []byte(hookLauncher), 0o700); err != nil {
		return err
	}
	for _, name := range []string{"LICENSE", "NOTICE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || len(data) == 0 {
			return fmt.Errorf("%s が欠けています", name)
		}
	}
	// 第三者の plugin 名には claude- を使えない。参照する agent/workflow/hook も揃える。
	if err := renameNamespace(dir); err != nil {
		return err
	}
	return validateEntrypoints(dir)
}

func renameNamespace(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("通常ファイルではありません: %s", path)
		}
		switch filepath.Ext(path) {
		case ".md", ".json", ".js", ".py":
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			if strings.Contains(text, "claude-security:") {
				return os.WriteFile(path, []byte(strings.ReplaceAll(text, "claude-security:", "security-uv:")), 0o600)
			}
		}
		return nil
	})
}

func replace(dir string, edit replacement) error {
	path := filepath.Join(dir, filepath.FromSlash(edit.path))
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if count := strings.Count(string(data), edit.old); count != edit.count {
		return fmt.Errorf("%s: 変換対象の件数が想定外です（期待 %d、実際 %d）", edit.path, edit.count, count)
	}
	return os.WriteFile(path, []byte(strings.ReplaceAll(string(data), edit.old, edit.new)), 0o600)
}

// JSON は文字列置換ではなく構造を保って変更し、元の metadata は残す。
func patchJSON(dir string) error {
	manifestPath := filepath.Join(dir, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest["name"] != "claude-security" {
		return errors.New("取得した plugin の名前が違います")
	}
	manifest["name"] = "security-uv"
	manifest["defaultEnabled"] = false
	if err := writeJSON(manifestPath, manifest); err != nil {
		return err
	}
	hooksPath := filepath.Join(dir, "hooks", "hooks.json")
	data, err = os.ReadFile(hooksPath)
	if err != nil {
		return err
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok {
		return errors.New("hook 設定の構造が想定外です")
	}
	count := 0
	for _, event := range []string{"PostToolUse", "PostToolUseFailure"} {
		rows, _ := hooks[event].([]any)
		for _, row := range rows {
			group, _ := row.(map[string]any)
			commands, _ := group["hooks"].([]any)
			for _, command := range commands {
				hook, _ := command.(map[string]any)
				if hook["if"] == "Bash(py* *claude-security*scripts/*.py *)" {
					hook["if"] = "Bash(" + runner + " *security-uv*scripts/*.py *)"
					count++
				}
			}
		}
	}
	if count != 2 {
		return fmt.Errorf("metrics hook が想定した2件ではありません: %d", count)
	}
	return writeJSON(hooksPath, document)
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// 解析用の interpreter 名とは区別し、command/例示/shebang の未変換を検出する。
var barePython = regexp.MustCompile("(?m)(^|[ \\t`\"(/])(?:python[0-9.]*|py)(?:[ \\t]+|$)")

func validateEntrypoints(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("通常ファイルではありません: %s", path)
		}
		ext := filepath.Ext(path)
		if ext != ".md" && ext != ".sh" && ext != ".js" && ext != ".py" && ext != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if ext == ".py" {
			// Python 本体のコメントや parser の文字列は起動命令ではない。
			text, _, _ = strings.Cut(text, "\n")
			if strings.HasPrefix(text, "#!") && text != "#!/usr/bin/env -S "+runner {
				return fmt.Errorf("%s に未対応の shebang があります", path)
			}
			return nil
		}
		text = strings.ReplaceAll(text, runner, "UV_PYTHON")
		text = strings.ReplaceAll(text, "uv run python3", "UV_PYTHON")
		if barePython.MatchString(text) {
			rel, _ := filepath.Rel(dir, path)
			return fmt.Errorf("%s に未対応の Python 実行入口があります", rel)
		}
		return nil
	})
}
