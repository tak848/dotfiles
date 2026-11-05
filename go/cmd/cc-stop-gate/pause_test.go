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

// CC_STOP_GATE_PAUSE_REJECT_KEYS のキーのどれかが false の結果では止めず、モデルに直させる。
// 読めない・形が違う・キーが無いときは今までどおり止める。
func TestPauseRejectKeys(t *testing.T) {
	t.Parallel()
	text := func(body string) []any { return []any{map[string]any{"type": "text", "text": body}} }
	tests := map[string]struct {
		response any
		stop     bool
	}{
		"rejected":             {map[string]any{"accepted": false, "reason": "too long"}, false},
		"accepted":             {map[string]any{"accepted": true}, true},
		"other key false":      {map[string]any{"ok": false}, false},
		"unlisted key false":   {map[string]any{"done": false}, true},
		"key missing":          {map[string]any{"reason": "x"}, true},
		"string false":         {map[string]any{"accepted": "false"}, true},
		"null":                 {map[string]any{"accepted": nil}, true},
		"zero":                 {map[string]any{"accepted": 0}, true},
		"nested only":          {map[string]any{"result": map[string]any{"accepted": false}}, true},
		"case differs":         {map[string]any{"Accepted": false}, true},
		"content rejected":     {text(`{"accepted":false,"reason":"too long"}`), false},
		"content accepted":     {text(`{"accepted":true}`), true},
		"content first text":   {[]any{map[string]any{"type": "image", "data": "x"}, map[string]any{"type": "text", "text": `{"accepted":false}`}}, false},
		"content later text":   {[]any{map[string]any{"type": "text", "text": `{"accepted":true}`}, map[string]any{"type": "text", "text": `{"accepted":false}`}}, true},
		"content not json":     {text("accepted: false"), true},
		"content json array":   {text(`[{"accepted":false}]`), true},
		"content no text":      {[]any{map[string]any{"type": "image", "data": "x"}}, true},
		"content empty":        {[]any{}, true},
		"content broken block": {[]any{"text"}, true},
		// MCP のツールが structuredContent を返すと、それを JSON にした文字列で届く。
		"structured rejected": {`{"accepted":false,"reason":"too long"}`, false},
		"structured accepted": {`{"accepted":true,"reason":"ok"}`, true},
		"string not json":     {"accepted: false", true},
		"string json array":   {`[{"accepted":false}]`, true},
		"string double":       {`"{\"accepted\":false}"`, true},
		"number response":     {0, true},
		"no response":         {nil, true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := postTool(pauseTool)
			input["tool_response"] = tt.response
			if tt.response == nil {
				delete(input, "tool_response")
			}
			env := envMap(map[string]string{pauseToolsEnv: pauseTool, pauseRejectKeysEnv: " ok, accepted ,"})
			if got := pause(t, env, input) != ""; got != tt.stop {
				t.Fatalf("stop = %v, want %v", got, tt.stop)
			}
		})
	}
	// 生の JSON でしか作れない形。
	for name, tt := range map[string]struct {
		raw  string
		stop bool
	}{
		// text の本文で重複したキーは、読めない形として止める。
		"content duplicate": {`{"hook_event_name":"PostToolUse","tool_name":"` + pauseTool + `","tool_response":[{"type":"text","text":"{\"accepted\":true,\"accepted\":false}"}]}`, true},
		// hook の入力そのものに重複したキーがあると入力全体が読めないので、今までどおり何も返さない。
		"input duplicate": {`{"hook_event_name":"PostToolUse","tool_name":"` + pauseTool + `","tool_response":{"accepted":true,"accepted":false}}`, false},
		// 切り詰めでサロゲートが割れた結果でも、キーを読んで判定する。
		"broken surrogate accepted": {`{"hook_event_name":"PostToolUse","tool_name":"` + pauseTool + `","tool_response":{"accepted":true,"stdout":"\ud83d"}}`, true},
		"broken surrogate rejected": {`{"hook_event_name":"PostToolUse","tool_name":"` + pauseTool + `","tool_response":{"accepted":false,"stdout":"\ud83d"}}`, false},
		"content broken surrogate":  {`{"hook_event_name":"PostToolUse","tool_name":"` + pauseTool + `","tool_response":[{"type":"text","text":"{\"accepted\":false,\"note\":\"\\ud83d\"}"}]}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := envMap(map[string]string{pauseToolsEnv: pauseTool, pauseRejectKeysEnv: "accepted"})
			if got := pause(t, env, tt.raw) != ""; got != tt.stop {
				t.Fatalf("stop = %v, want %v", got, tt.stop)
			}
		})
	}
}

// キーの一覧が空なら、結果の中身に関係なく今までどおり止める。
func TestPauseRejectKeysUnset(t *testing.T) {
	t.Parallel()
	input := postTool(pauseTool)
	input["tool_response"] = map[string]any{"accepted": false}
	for name, vars := range map[string]map[string]string{
		"unset":           {pauseToolsEnv: pauseTool},
		"empty":           {pauseToolsEnv: pauseTool, pauseRejectKeysEnv: ""},
		"separators only": {pauseToolsEnv: pauseTool, pauseRejectKeysEnv: " , "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if pause(t, envMap(vars), input) == "" {
				t.Fatal("must stop when no reject keys are configured")
			}
		})
	}
	// 一覧に無いツールや subagent 内のツールは、キーに関係なく止めない。
	other := postTool("Read")
	other["tool_response"] = map[string]any{"accepted": true}
	if out := pause(t, envMap(map[string]string{pauseToolsEnv: pauseTool, pauseRejectKeysEnv: "accepted"}), other); out != "" {
		t.Fatalf("unlisted tool must not stop: %s", out)
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
