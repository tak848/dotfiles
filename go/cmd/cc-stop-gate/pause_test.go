package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	pauseTool   = "mcp__example__wait_reply"
	testSession = "session-a"
)

func pauseEnv(t *testing.T, tools string) (lookupEnv, string) {
	t.Helper()
	state := t.TempDir()
	return envMap(map[string]string{pauseToolsEnv: tools, "XDG_STATE_HOME": state}), filepath.Join(state, "cc-stop-gate", "pause")
}

// hook は pause-marker に 1 件の hook 入力を渡す。string はそのままの JSON として渡す。
func hook(t *testing.T, env lookupEnv, input any) {
	t.Helper()
	data, ok := input.(string)
	if !ok {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		data = string(raw)
	}
	if code := runMarker(strings.NewReader(data), env); code != 0 {
		t.Fatalf("exit = %d", code)
	}
}

func postTool(session, tool string) map[string]any {
	return map[string]any{"hook_event_name": "PostToolUse", "session_id": session, "tool_name": tool, "tool_response": map[string]any{"ok": true}}
}

func failTool(session, tool string) map[string]any {
	return map[string]any{"hook_event_name": "PostToolUseFailure", "session_id": session, "tool_name": tool, "error": "Exit code 1"}
}

func inSubagent(fields map[string]any) map[string]any {
	fields["agent_id"] = "agent-1"
	fields["agent_type"] = "Explore"
	return fields
}

func prompt(session string) map[string]any {
	return map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": session, "prompt": "続きをお願いします"}
}

// stop は署名の無い報告で Stop を 1 回実行し、差し戻されたかを返す。
// 署名が無いので、判定に進めば必ず差し戻される。
func stop(t *testing.T, env lookupEnv, session string) bool {
	t.Helper()
	data, err := json.Marshal(map[string]string{
		"hook_event_name": "Stop", "permission_mode": "default", "cwd": "/example/repo",
		"session_id": session, "last_assistant_message": "結果を共有しました。",
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	checked := false
	check := func(context.Context, string, []string) []string { checked = true; return nil }
	if code := run(bytes.NewReader(data), &out, env, check); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if blocked := out.Len() != 0; blocked != checked {
		t.Fatalf("blocked = %v but git checked = %v: %s", blocked, checked, &out)
	}
	return out.Len() != 0
}

func TestPauseToolEndsTurn(t *testing.T) {
	t.Parallel()
	env, dir := pauseEnv(t, pauseTool)
	hook(t, env, postTool(testSession, pauseTool))
	if _, err := os.Stat(filepath.Join(dir, testSession)); err != nil {
		t.Fatalf("marker not written: %v", err)
	}
	if stop(t, env, testSession) {
		t.Fatal("turn ending with a pause tool must pass")
	}
	if _, err := os.Stat(filepath.Join(dir, testSession)); !os.IsNotExist(err) {
		t.Fatalf("marker must be consumed: %v", err)
	}
	// マーカーは 1 回の Stop で消えるので、次の turn は通常どおり判定される。
	if !stop(t, env, testSession) {
		t.Fatal("next turn must be judged")
	}
}

func TestPauseMarkerSequences(t *testing.T) {
	t.Parallel()
	// 切り詰めでサロゲートが割れた tool_response。読めないとマーカーを消せない。
	brokenSurrogate := `{"hook_event_name":"PostToolUse","session_id":"` + testSession + `","tool_name":"Bash","tool_response":{"stdout":"\ud83d"}}`
	tests := map[string]struct {
		steps   []any
		blocked bool
	}{
		"pause then other tool":        {[]any{postTool(testSession, pauseTool), postTool(testSession, "Read")}, true},
		"pause then failing tool":      {[]any{postTool(testSession, pauseTool), failTool(testSession, "Bash")}, true},
		"pause tool fails":             {[]any{postTool(testSession, pauseTool), failTool(testSession, pauseTool)}, true},
		"other tool then pause":        {[]any{postTool(testSession, "Read"), postTool(testSession, pauseTool)}, false},
		"pause then prompt":            {[]any{postTool(testSession, pauseTool), prompt(testSession)}, true},
		"subagent tool after pause":    {[]any{postTool(testSession, pauseTool), inSubagent(postTool(testSession, "Read"))}, false},
		"subagent failure after pause": {[]any{postTool(testSession, pauseTool), inSubagent(failTool(testSession, "Bash"))}, false},
		"pause in subagent":            {[]any{inSubagent(postTool(testSession, pauseTool))}, true},
		"other session pause":          {[]any{postTool("session-b", pauseTool)}, true},
		"other session clears nothing": {[]any{postTool(testSession, pauseTool), postTool("session-b", "Read"), prompt("session-b")}, false},
		"prefix is not a match":        {[]any{postTool(testSession, "mcp__example__wait")}, true},
		"case differs":                 {[]any{postTool(testSession, strings.ToUpper(pauseTool))}, true},
		"unrelated event keeps marker": {[]any{postTool(testSession, pauseTool), map[string]any{"hook_event_name": "Notification", "session_id": testSession}}, false},
		"missing session keeps marker": {[]any{postTool(testSession, pauseTool), postTool("", "Read")}, false},
		"no tool name clears":          {[]any{postTool(testSession, pauseTool), map[string]any{"hook_event_name": "PostToolUse", "session_id": testSession}}, true},
		"broken surrogate clears":      {[]any{postTool(testSession, pauseTool), brokenSurrogate}, true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env, _ := pauseEnv(t, pauseTool)
			for _, step := range tt.steps {
				hook(t, env, step)
			}
			if got := stop(t, env, testSession); got != tt.blocked {
				t.Fatalf("blocked = %v, want %v", got, tt.blocked)
			}
		})
	}
}

// 機能が無効なら、マーカーを書かず、既存のマーカーも見ずに従来どおり判定する。
func TestPauseDisabledKeepsBehavior(t *testing.T) {
	t.Parallel()
	for name, tools := range map[string]*string{"unset": nil, "empty": new(""), "separators only": new(" , ,")} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			vars := map[string]string{"XDG_STATE_HOME": state}
			if tools != nil {
				vars[pauseToolsEnv] = *tools
			}
			env := envMap(vars)
			if code := runMarker(fatalReader{t}, env); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			if entries, _ := os.ReadDir(state); len(entries) != 0 {
				t.Fatalf("state dir touched: %v", entries)
			}
			marker := filepath.Join(state, "cc-stop-gate", "pause", testSession)
			if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(marker, []byte(pauseTool+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if !stop(t, env, testSession) {
				t.Fatal("stale marker must be ignored while the feature is disabled")
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("marker must be left untouched: %v", err)
			}
		})
	}
}

func TestPauseMarkerRespectsOptOut(t *testing.T) {
	t.Parallel()
	env := envMap(map[string]string{"CC_STOP_GATE": "0", pauseToolsEnv: pauseTool, "XDG_STATE_HOME": t.TempDir()})
	if code := runMarker(fatalReader{t}, env); code != 0 {
		t.Fatalf("exit = %d", code)
	}
}

func TestPauseToolsParsing(t *testing.T) {
	t.Parallel()
	got := pauseTools(envMap(map[string]string{pauseToolsEnv: " mcp__a__x , ,mcp__b__y,MCP__A__X,mcp__a__x "}))
	want := map[string]bool{"mcp__a__x": true, "mcp__b__y": true, "MCP__A__X": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tools = %v, want %v", got, want)
	}
}

func TestMarkerPath(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"", ".", "..", "../x", "a/b", `a\b`, "a b", "セッション", strings.Repeat("a", maxSessionIDBytes+1)} {
		if _, ok := markerPath(envMap(map[string]string{"XDG_STATE_HOME": "/state"}), id); ok {
			t.Errorf("session id %q must be rejected", id)
		}
	}
	tests := map[string]struct {
		env  map[string]string
		want string
	}{
		"xdg":          {map[string]string{"XDG_STATE_HOME": "/state", "HOME": "/home/u"}, "/state/cc-stop-gate/pause/0a1b-2c_3.d"},
		"relative xdg": {map[string]string{"XDG_STATE_HOME": "state", "HOME": "/home/u"}, "/home/u/.local/state/cc-stop-gate/pause/0a1b-2c_3.d"},
		"home":         {map[string]string{"HOME": "/home/u"}, "/home/u/.local/state/cc-stop-gate/pause/0a1b-2c_3.d"},
		"no home":      {map[string]string{"HOME": ""}, ""},
	}
	for name, tt := range tests {
		got, ok := markerPath(envMap(tt.env), "0a1b-2c_3.d")
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("%s: path = %q, %v, want %q", name, got, ok, tt.want)
		}
	}
}

// 置き場を作れなくても無出力・exit 0 で終え、Stop は通常どおり判定する。
func TestPauseMarkerWriteFailure(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	env := envMap(map[string]string{pauseToolsEnv: pauseTool, "XDG_STATE_HOME": blocker})
	hook(t, env, postTool(testSession, pauseTool))
	hook(t, env, prompt(testSession))
	if !stop(t, env, testSession) {
		t.Fatal("unwritable state dir must not disable the gate")
	}
}

func TestPauseMarkerInvalidInput(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"empty": "", "malformed": "{", "null": "null", "array": "[]", "two objects": "{}{}",
		"duplicate": `{"hook_event_name":"PostToolUse","session_id":"session-a","tool_name":"Read","tool_name":"` + pauseTool + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env, dir := pauseEnv(t, pauseTool)
			if code := runMarker(strings.NewReader(text), env); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("marker dir created: %v", err)
			}
		})
	}
}

// 入力の不備で通す Stop でもマーカーを消費し、次の turn に残さない。
func TestMalformedStopConsumesMarker(t *testing.T) {
	t.Parallel()
	env, dir := pauseEnv(t, pauseTool)
	hook(t, env, postTool(testSession, pauseTool))
	var out bytes.Buffer
	payload := `{"hook_event_name":"Stop","session_id":"session-a","permission_mode":"default"}`
	if code := run(strings.NewReader(payload), &out, env, nil); code != 0 || out.Len() != 0 {
		t.Fatalf("exit = %d, output = %q", code, &out)
	}
	if _, err := os.Stat(filepath.Join(dir, testSession)); !os.IsNotExist(err) {
		t.Fatalf("marker must be consumed: %v", err)
	}
}

// fatalReader は、stdin を読まずに終えるべき経路で読まれたら失敗させる。
type fatalReader struct{ t *testing.T }

func (r fatalReader) Read([]byte) (int, error) {
	r.t.Error("stdin must not be read")
	return 0, os.ErrClosed
}
