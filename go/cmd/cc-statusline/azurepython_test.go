package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Azure CLI 用の Python は run_onchange スクリプトが uv で入れ、mise の [env] AZ_PYTHON が
// その実行ファイルを指す。Renovate がスクリプト側のマイナーバージョンだけを上げると、
// 新しいマシンでは AZ_PYTHON の指す python3.X が存在せず az が起動しない。既存のマシンは
// 古い版が残っているので気づけないため、ここで両者のマイナーバージョンと置き場所の一致を検証する。
const azurePythonScript = "run_onchange_after_15-azure-cli-python.sh.tmpl"

var (
	azurePythonVersionRe = regexp.MustCompile(`(?m)^AZURE_CLI_PYTHON_VERSION="(\d+\.\d+)\.\d+"`)
	azurePythonBinDirRe  = regexp.MustCompile(`(?m)^export UV_PYTHON_BIN_DIR="\$\{XDG_DATA_HOME\}(/[^"]*)"`)
	// XDG_DATA_HOME は mise のテンプレート（{{ env.XDG_DATA_HOME | default(...) }}）なので、その後ろを比べる
	azPythonEnvRe = regexp.MustCompile(`(?m)^AZ_PYTHON = '\{\{[^}]*\}\}(/[^']*)/python(\d+\.\d+)'`)
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

	m := azurePythonVersionRe.FindSubmatch(script)
	if m == nil {
		t.Fatalf("%s has no AZURE_CLI_PYTHON_VERSION=\"X.Y.Z\" line", azurePythonScript)
	}
	installed := string(m[1])

	m = azurePythonBinDirRe.FindSubmatch(script)
	if m == nil {
		t.Fatalf("%s has no export UV_PYTHON_BIN_DIR=\"${XDG_DATA_HOME}/...\" line", azurePythonScript)
	}
	binDir := string(m[1])

	m = azPythonEnvRe.FindSubmatch(config)
	if m == nil {
		t.Fatal("dot_config/mise/config.toml has no AZ_PYTHON = '{{ env.XDG_DATA_HOME ... }}/.../pythonX.Y' line")
	}
	expectedDir, expected := string(m[1]), string(m[2])

	if installed != expected {
		t.Errorf("%s installs Python %s but AZ_PYTHON points to python%s", azurePythonScript, installed, expected)
	}
	if binDir != expectedDir {
		t.Errorf("%s installs Python into $XDG_DATA_HOME%s but AZ_PYTHON points into $XDG_DATA_HOME%s", azurePythonScript, binDir, expectedDir)
	}
}
