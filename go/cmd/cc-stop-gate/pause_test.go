package main

import (
	"bytes"
	"encoding/json/v2"
	"os"
	"reflect"
	"strings"
	"testing"
)

const pauseTool = "mcp__example__wait_reply"

func postTool(tool string) map[string]any {
	return map[string]any{"hook_event_name": "PostToolUse", "session_id": "session-a", "tool_name": tool, "tool_response": map[string]any{"ok": true}}
}

// pause は pause-tool に 1 件の hook 入力を渡し、出力を返す。string はそのままの JSON として渡す。
func pause(t *testing.T, env lookupEnv, input any) string {
	t.Helper()
	data, ok := input.(string)
	if !ok {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		data = string(raw)
	}
	var out bytes.Buffer
	if code := runPause(strings.NewReader(data), &out, env); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	return out.String()
}

func TestPauseTool(t *testing.T) {
	t.Parallel()
	subagent := postTool(pauseTool)
	subagent["agent_id"] = "agent-1"
	subagent["agent_type"] = "Explore"
	failure := postTool(pauseTool)
	failure["hook_event_name"] = "PostToolUseFailure"
	tests := map[string]struct {
		input any
		stop  bool
	}{
		"listed tool":        {postTool(pauseTool), true},
		"other tool":         {postTool("Read"), false},
		"prefix":             {postTool("mcp__example__wait"), false},
		"case differs":       {postTool(strings.ToUpper(pauseTool)), false},
		"no tool name":       {map[string]any{"hook_event_name": "PostToolUse"}, false},
		"inside subagent":    {subagent, false},
		"failed listed tool": {failure, false},
		"other event":        {map[string]any{"hook_event_name": "UserPromptSubmit", "tool_name": pauseTool}, false},
		// 切り詰めでサロゲートが割れた tool_response でも読める。
		"broken surrogate": {`{"hook_event_name":"PostToolUse","tool_name":"` + pauseTool + `","tool_response":{"stdout":"\ud83d"}}`, true},
		"malformed":        {"{", false},
		"null":             {"null", false},
		"array":            {"[]", false},
		"two objects":      {"{}{}", false},
		"duplicate":        {`{"hook_event_name":"PostToolUse","tool_name":"Read","tool_name":"` + pauseTool + `"}`, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := envMap(map[string]string{pauseToolsEnv: " Read2, " + pauseTool + " ,"})
			out := pause(t, env, tt.input)
			if !tt.stop {
				if out != "" {
					t.Fatalf("unexpected output: %s", out)
				}
				return
			}
			// continue: false だけを返す。stopReason はユーザーに表示され会話にも残るので付けない。
			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("invalid output %q: %v", out, err)
			}
			if !reflect.DeepEqual(got, map[string]any{"continue": false}) {
				t.Fatalf("output = %v", got)
			}
		})
	}
}

// 機能が無効なら、stdin を読まずに何も返さない。
func TestPauseToolDisabled(t *testing.T) {
	t.Parallel()
	for name, vars := range map[string]map[string]string{
		"unset":           nil,
		"empty":           {pauseToolsEnv: ""},
		"separators only": {pauseToolsEnv: " , ,"},
		"gate opt-out":    {pauseToolsEnv: pauseTool, "CC_STOP_GATE": "0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if code := runPause(fatalReader{t}, &out, envMap(vars)); code != 0 || out.Len() != 0 {
				t.Fatalf("exit = %d, output = %q", code, &out)
			}
		})
	}
}

func TestPauseToolWriteFailure(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(postTool(pauseTool))
	if err != nil {
		t.Fatal(err)
	}
	if code := runPause(bytes.NewReader(data), failingWriter{}, envMap(map[string]string{pauseToolsEnv: pauseTool})); code != 0 {
		t.Fatalf("exit = %d", code)
	}
}

// Stop ゲート自体は変数に関係なく、これまでどおり判定する。
func TestStopIgnoresPauseTools(t *testing.T) {
	t.Parallel()
	payload := `{"hook_event_name":"Stop","permission_mode":"plan","last_assistant_message":"結果を共有しました。"}`
	var out bytes.Buffer
	if code := run(strings.NewReader(payload), &out, envMap(map[string]string{pauseToolsEnv: pauseTool}), nil); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), `"block"`) {
		t.Fatalf("Stop must still be judged: %q", &out)
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

// fatalReader は、stdin を読まずに終えるべき経路で読まれたら失敗させる。
type fatalReader struct{ t *testing.T }

func (r fatalReader) Read([]byte) (int, error) {
	r.t.Error("stdin must not be read")
	return 0, os.ErrClosed
}
