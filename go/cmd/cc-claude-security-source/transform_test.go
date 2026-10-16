package main

import (
	"encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// 上流の本体は fixture に含めず、変換の境界だけを自作データで再現する。
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"README.md":                                  "fixture intro\n## Installation\nfixture original install\n## Getting started\nfixture usage\n",
		".claude-plugin/plugin.json":                 `{"name":"claude-security","version":"fixture","author":{"name":"fixture"}}`,
		"LICENSE":                                    "fixture license\n",
		"NOTICE.md":                                  "fixture notice\n",
		"hooks/hooks.sh":                             "#!/bin/sh\nexit 88\n",
		"hooks/hooks.py":                             "#!/usr/bin/env python3\n    if words[:2] == [PY_LAUNCHER, PY_VERSION_OPTION]:\n        pass\n",
		"skills/claude-security/role.md":             "fixture role\n",
		"agents/patch-generator.md":                  "fixture generator\n",
		"agents/patch-verifier.md":                   "fixture verifier\n",
		"skills/claude-security/specs/patch-spec.md": "python -m pytest tests/ -q\n",
		"workflows/scan.js":                          "Run save_result.py\nRun save_result.py\nrun write_scan_meta.py\nclaude-security:scan\n",
	}
	for _, name := range []string{"render_report", "save_result", "write_scan_meta", "patch_artifacts"} {
		files["scripts/"+name+".py"] = "#!/usr/bin/env python3\n# fixture\n"
		files["skills/claude-security/SKILL.md"] += `  - Bash(python3 "${CLAUDE_PLUGIN_ROOT}/scripts/` + name + `.py" *)` + "\n"
	}
	files["scripts/save_result.py"] += "'run render_report.py'\n'run save_result.py'\n"
	for path, count := range map[string]int{"scan-codebase": 4, "scan-changes": 4, "suggest-patches": 5} {
		files["skills/claude-security/jobs/"+path+".md"] = strings.Repeat(`python3 "SCRIPTS/fixture.py"`+"\n", count)
	}
	hook := `{"if":"Bash(py* *claude-security*scripts/*.py *)","command":"sh hook metrics","async":true}`
	files["hooks/hooks.json"] = `{"description":"fixture","hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[` + hook + `]}],"PostToolUseFailure":[{"matcher":"Bash","hooks":[` + hook + `]}]}}`
	for path, text := range files {
		put(t, filepath.Join(dir, filepath.FromSlash(path)), text, 0o700)
	}
	return dir
}

func put(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTransform(t *testing.T) {
	dir := fixture(t)
	if err := transform(dir); err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, ".claude-plugin/plugin.json"))), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["defaultEnabled"] != false || manifest["version"] != "fixture" || manifest["name"] != "security-uv" {
		t.Fatalf("manifest = %v", manifest)
	}
	if got := read(t, filepath.Join(dir, "LICENSE")); got != "fixture license\n" {
		t.Fatal("LICENSE が変わっています")
	}
	if got := read(t, filepath.Join(dir, "NOTICE.md")); got != "fixture notice\n" {
		t.Fatal("NOTICE が変わっています")
	}
	if got := read(t, filepath.Join(dir, "hooks/hooks.sh")); got != hookLauncher {
		t.Fatal("uv launcher が生成されていません")
	}
	for _, path := range []string{"hooks/hooks.py", "scripts/render_report.py", "scripts/save_result.py", "scripts/write_scan_meta.py", "scripts/patch_artifacts.py"} {
		if !strings.HasPrefix(read(t, filepath.Join(dir, path)), "#!/usr/bin/env -S "+runner+"\n") {
			t.Errorf("%s の shebang が uv 経由ではありません", path)
		}
	}
	hooks := read(t, filepath.Join(dir, "hooks/hooks.json"))
	if strings.Count(hooks, "Bash("+runner+" *security-uv*scripts/*.py *)") != 2 || !strings.Contains(hooks, `"async":true`) {
		t.Fatal("metrics hook の変換が不正です")
	}
	parser := read(t, filepath.Join(dir, "hooks/hooks.py"))
	if !strings.Contains(parser, "if words[:len(uv_prefix)] != uv_prefix:") || !strings.Contains(parser, "words = words[len(uv_prefix) - 1:]") {
		t.Fatal("metrics の完全一致 prefix 検査がありません")
	}
	for _, path := range []string{"skills/claude-security/role.md", "agents/patch-generator.md", "agents/patch-verifier.md"} {
		if !strings.Contains(read(t, filepath.Join(dir, path)), pythonPolicy) {
			t.Errorf("%s に Python 規約がありません", path)
		}
	}
	if !strings.Contains(read(t, filepath.Join(dir, "scripts/save_result.py")), runner+` \"SCRIPTS/render_report.py\"`) {
		t.Fatal("次の helper の指示が未変換です")
	}
	readme := read(t, filepath.Join(dir, "README.md"))
	if !strings.Contains(readme, localInstallation) || !strings.Contains(readme, "fixture intro") || !strings.Contains(readme, "fixture usage") || strings.Contains(readme, "fixture original install") {
		t.Fatal("README の導入節が自前版に揃っていません")
	}
	workflow := read(t, filepath.Join(dir, "workflows/scan.js"))
	if strings.Contains(workflow, "claude-security:") || !strings.Contains(workflow, "security-uv:scan") {
		t.Fatal("workflow の namespace が揃っていません")
	}
}

func TestTransformRejectsUnexpectedInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, string)
	}{
		{"missing", func(t *testing.T, d string) {
			t.Helper()
			if err := os.Remove(filepath.Join(d, "agents/patch-verifier.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing-license", func(t *testing.T, d string) {
			t.Helper()
			if err := os.Remove(filepath.Join(d, "LICENSE")); err != nil {
				t.Fatal(err)
			}
		}},
		{"count", func(t *testing.T, d string) {
			put(t, filepath.Join(d, "skills/claude-security/SKILL.md"), "no commands\n", 0o600)
		}},
		{"unknown-shell", func(t *testing.T, d string) {
			put(t, filepath.Join(d, "scripts/new.sh"), "#!/bin/sh\npython3 -c 'exit(0)'\n", 0o700)
		}},
		{"absolute-python", func(t *testing.T, d string) {
			put(t, filepath.Join(d, "scripts/new.sh"), "#!/bin/sh\n/usr/bin/python3 -c 'exit(0)'\n", 0o700)
		}},
		{"unknown-shebang", func(t *testing.T, d string) {
			put(t, filepath.Join(d, "scripts/new.py"), "#!/usr/bin/env python3\n", 0o700)
		}},
		{"unknown-example", func(t *testing.T, d string) {
			put(t, filepath.Join(d, "agents/new.md"), "Run `python -m pytest`.\n", 0o600)
		}},
		{"manifest", func(t *testing.T, d string) {
			put(t, filepath.Join(d, ".claude-plugin/plugin.json"), `{"name":"other"}`, 0o600)
		}},
		{"metrics", func(t *testing.T, d string) { put(t, filepath.Join(d, "hooks/hooks.json"), `{"hooks":{}}`, 0o600) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t)
			tc.change(t, dir)
			if err := transform(dir); err == nil {
				t.Fatal("未知の構造を成功扱いしました")
			}
		})
	}
}

func TestHookLauncher(t *testing.T) {
	for _, tc := range []struct {
		name, mode, tip string
		code            int
		noUV, skip      bool
	}{
		{name: "banner", mode: "banner"},
		{name: "metrics", mode: "metrics"},
		{name: "unanswered", mode: "unanswered"},
		{name: "tip", mode: "tip"},
		{name: "tip-off", mode: "tip", tip: "off", skip: true},
		{name: "tip-case", mode: "tip", tip: "OFF", skip: true},
		{name: "tip-ci", mode: "tip", skip: true},
		{name: "uv-failure", mode: "banner", code: 17},
		{name: "uv-missing", mode: "banner", code: 127, noUV: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "path with spaces")
			bin := filepath.Join(dir, "bin")
			launcher := filepath.Join(dir, "hooks/hooks.sh")
			trace := filepath.Join(dir, "trace")
			put(t, launcher, hookLauncher, 0o700)
			if !tc.noUV {
				put(t, filepath.Join(bin, "uv"), "#!/bin/sh\nprintf '%s\\000' \"$@\" >\"$TRACE\"\n/bin/cat >\"$TRACE.stdin\"\nprintf '%s' \"${GIT_DIR-unset}\" >\"$TRACE.git\"\nexit \"$UV_EXIT\"\n", 0o700)
			}
			for _, name := range []string{"python", "python3", "py"} {
				put(t, filepath.Join(bin, name), "#!/bin/sh\nprintf direct >\"$TRACE.python\"\nexit 99\n", 0o700)
			}
			if err := os.Symlink("/usr/bin/dirname", filepath.Join(bin, "dirname")); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/sh", launcher, tc.mode, "push")
			cmd.Stdin = strings.NewReader("{\"fixture\":true}\n")
			ci := ""
			if tc.name == "tip-ci" {
				ci = "true"
			}
			cmd.Env = []string{"PATH=" + bin, "TRACE=" + trace, "UV_EXIT=" + stringCode(tc.code), "CLAUDE_PLUGIN_DATA=" + dir, "CLAUDE_SECURITY_SCAN_TIP=" + tc.tip, "CI=" + ci, "GIT_DIR=unrelated"}
			out, err := cmd.CombinedOutput()
			gotCode := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					gotCode = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if gotCode != tc.code {
				t.Fatalf("exit = %d, want %d: %s", gotCode, tc.code, out)
			}
			if _, err := os.Stat(trace + ".python"); !os.IsNotExist(err) {
				t.Fatal("Python を直接起動しました")
			}
			if tc.skip || tc.noUV {
				if _, err := os.Stat(trace); !os.IsNotExist(err) {
					t.Fatal("uv を実行しました")
				}
				return
			}
			want := append(strings.Fields(runner)[1:], filepath.Join(dir, "hooks/hooks.py"))
			switch tc.mode {
			case "banner":
				want = append(want, "banner", dir)
			case "tip":
				want = append(want, "tip", "push", dir, tc.tip)
			default:
				want = append(want, tc.mode, "push")
			}
			got := strings.Split(strings.TrimSuffix(read(t, trace), "\x00"), "\x00")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("argv = %q, want %q", got, want)
			}
			if read(t, trace+".stdin") != "{\"fixture\":true}\n" {
				t.Fatal("stdin が変わっています")
			}
			if tc.mode == "tip" && read(t, trace+".git") != "unset" {
				t.Fatal("Git 環境が残っています")
			}
		})
	}
}

func stringCode(code int) string {
	if code == 17 {
		return "17"
	}
	return "0"
}

// 実ソースは明示した専用一時領域から読むだけで、CI で外部コードを取得・実行しない。
func TestPinnedUpstream(t *testing.T) {
	source := os.Getenv("CC_SECURITY_UPSTREAM_FIXTURE")
	if source == "" {
		t.Skip("固定版の実ソース検証は手動指定時だけ実行")
	}
	dir := t.TempDir()
	if err := copyTree(source, dir); err != nil {
		t.Fatal(err)
	}
	if err := transform(dir); err != nil {
		t.Fatal(err)
	}
}
