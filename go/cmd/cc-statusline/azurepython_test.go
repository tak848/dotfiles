package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Azure CLI 用の Python は run_onchange スクリプトが uv で UV_PYTHON_BIN_DIR に入れ、版を含まない
// リンクをそこへ張る。mise の [env] AZ_PYTHON はそのリンクを指す。片方だけ置き場所やリンク名を
// 変えると az が起動しなくなるが、既存のマシンには古いリンクやファイルが残っていて気づけないため、
// ここで両者の一致を検証する。
const azurePythonScript = "run_onchange_after_15-azure-cli-python.sh.tmpl"

var (
	azurePythonBinDirRe = regexp.MustCompile(`(?m)^export UV_PYTHON_BIN_DIR="\$\{XDG_DATA_HOME\}(/[^"]*)"`)
	azurePythonLinkRe   = regexp.MustCompile(`(?m)^ln -sfn "python\$\{py_ver\}" "\$\{UV_PYTHON_BIN_DIR\}/([^"/]+)"`)
	// XDG_DATA_HOME は mise のテンプレート（{{ env.XDG_DATA_HOME | default(...) }}）なので、その後ろを比べる
	azPythonEnvRe = regexp.MustCompile(`(?m)^AZ_PYTHON = '\{\{[^}]*\}\}(/[^']*)/([^'/]+)'`)
)

func TestAzurePythonMatchesAZPython(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	script, err := os.ReadFile(filepath.Join(root, azurePythonScript))
	if err != nil {
		t.Skipf("%s not found: %v", azurePythonScript, err)
	}
	config, err := os.ReadFile(filepath.Join(root, "dot_config", "mise", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	m := azurePythonBinDirRe.FindSubmatch(script)
	if m == nil {
		t.Fatalf("%s has no export UV_PYTHON_BIN_DIR=\"${XDG_DATA_HOME}/...\" line", azurePythonScript)
	}
	binDir := string(m[1])

	m = azurePythonLinkRe.FindSubmatch(script)
	if m == nil {
		t.Fatalf("%s has no ln -sfn \"python${py_ver}\" \"${UV_PYTHON_BIN_DIR}/<name>\" line", azurePythonScript)
	}
	link := string(m[1])

	m = azPythonEnvRe.FindSubmatch(config)
	if m == nil {
		t.Fatal("dot_config/mise/config.toml has no AZ_PYTHON = '{{ env.XDG_DATA_HOME ... }}/.../<name>' line")
	}
	expectedDir, expectedLink := string(m[1]), string(m[2])

	if binDir != expectedDir {
		t.Errorf("%s installs Python into $XDG_DATA_HOME%s but AZ_PYTHON points into $XDG_DATA_HOME%s", azurePythonScript, binDir, expectedDir)
	}
	if link != expectedLink {
		t.Errorf("%s links %s but AZ_PYTHON points to %s", azurePythonScript, link, expectedLink)
	}
}
