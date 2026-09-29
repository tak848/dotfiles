package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSignature は、実際の signature と同じく先頭付近に種別の語を持つ値を作る。
func fakeSignature(kind string) string {
	raw := append([]byte{0x08, 0x04, 0x12, 0x10, 0x0a, 0x10, 0x08, 0x12, 0x18, 0x02, 0x38, 0x01, 0x42, byte(len(kind))}, kind...)
	raw = append(raw, bytes.Repeat([]byte{0xa5}, 200)...)
	return base64.StdEncoding.EncodeToString(raw)
}

func line(t *testing.T, id string, blocks ...map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"id": id, "model": "claude-opus-5-5", "content": blocks},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func thinking(kind, text string) map[string]any {
	return map[string]any{"type": "thinking", "thinking": text, "signature": fakeSignature(kind)}
}

func toolUse(id string) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{}}
}

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hookInput(t *testing.T, event, transcript, agentID string, ids ...string) []byte {
	t.Helper()
	var calls []map[string]any
	for _, id := range ids {
		// tool_response は検査に使わない。割れたサロゲートを含んでも読めることを確かめる。
		calls = append(calls, map[string]any{"tool_name": "Bash", "tool_use_id": id, "tool_response": "x"})
	}
	obj := map[string]any{"hook_event_name": event, "transcript_path": transcript, "tool_calls": calls}
	if agentID != "" {
		obj["agent_id"] = agentID
	}
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Replace(data, []byte(`"x"`), []byte(`"\ud800"`), 1)
}

func noEnv(string) (string, bool) { return "", false }

func TestSignatureKind(t *testing.T) {
	t.Parallel()
	for sig, want := range map[string]string{
		fakeSignature("narration"): "narration",
		fakeSignature("thinking"):  "thinking",
		"":                         "",
		"%%%%not base64%%%%":       "",
		base64.StdEncoding.EncodeToString([]byte("unrelated bytes")): "",
		// 実際の transcript にある signature の先頭（通常の推論）。
		"CAQSrQcKEAgSGAI4AUIIdGhpbmtpbmc=": "thinking",
	} {
		if got := signatureKind(sig); got != want {
			t.Errorf("signatureKind(%q) = %q, want %q", sig, got, want)
		}
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	transcript := writeTranscript(t,
		`{"type":"user","message":{"role":"user","content":"依頼"}}`+"\n",
		// 前の batch。要約されたブロックがあっても、今の batch ではないので対象外。
		line(t, "msg_old", thinking("narration", "前の報告")),
		line(t, "msg_old", toolUse("toolu_old")),
		// 今の batch。Claude Code は 1 ブロックずつ別の行に書く。
		line(t, "msg_now", thinking("thinking", "")),
		line(t, "msg_now", thinking("narration", "調査結果:\n原因は A でした\x1b[31m")),
		line(t, "msg_now", toolUse("toolu_a")),
		line(t, "msg_now", toolUse("toolu_b")),
		// 通常の推論だけのメッセージ。
		line(t, "msg_plain", thinking("thinking", "")),
		line(t, "msg_plain", toolUse("toolu_plain")),
		"{broken json\n",
		// 同じ narration でも display が omitted なら本文は空。
		line(t, "msg_omitted", thinking("narration", "")),
		line(t, "msg_omitted", toolUse("toolu_omitted")),
	)

	tests := map[string]struct {
		input    []byte
		env      map[string]string
		contains []string
		excludes []string
	}{
		"detected": {
			input:    hookInput(t, "PostToolBatch", transcript, "", "toolu_a", "toolu_b"),
			contains: []string{"（1 件）", "- 調査結果: 原因は A でした", "ターン最後の text"},
			excludes: []string{"前の報告", "\x1b", "\n原因"},
		},
		"omitted summary": {
			input:    hookInput(t, "PostToolBatch", transcript, "", "toolu_omitted"),
			contains: []string{"（1 件）", "画面には何も表示されていない"},
		},
		"plain thinking":   {input: hookInput(t, "PostToolBatch", transcript, "", "toolu_plain")},
		"not yet written":  {input: hookInput(t, "PostToolBatch", transcript, "", "toolu_future")},
		"subagent":         {input: hookInput(t, "PostToolBatch", transcript, "agent-1", "toolu_a")},
		"wrong event":      {input: hookInput(t, "PostToolUse", transcript, "", "toolu_a")},
		"relative path":    {input: hookInput(t, "PostToolBatch", "session.jsonl", "", "toolu_a")},
		"missing file":     {input: hookInput(t, "PostToolBatch", filepath.Join(t.TempDir(), "none.jsonl"), "", "toolu_a")},
		"no tool calls":    {input: hookInput(t, "PostToolBatch", transcript, "")},
		"malformed input":  {input: []byte("{")},
		"empty input":      {input: nil},
		"opted out":        {input: hookInput(t, "PostToolBatch", transcript, "", "toolu_a"), env: map[string]string{"CC_NARRATION_CHECK": " Off "}},
		"not opted out by": {input: hookInput(t, "PostToolBatch", transcript, "", "toolu_a"), env: map[string]string{"CC_NARRATION_CHECK": "1"}, contains: []string{"（1 件）"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := noEnv
			if tt.env != nil {
				env = func(key string) (string, bool) { v, ok := tt.env[key]; return v, ok }
			}
			var out bytes.Buffer
			if code := run(bytes.NewReader(tt.input), &out, env); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			if len(tt.contains) == 0 {
				if out.Len() != 0 {
					t.Fatalf("unexpected output: %s", out.String())
				}
				return
			}
			var got output
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("invalid output %q: %v", out.String(), err)
			}
			if got.HookSpecificOutput.HookEventName != "PostToolBatch" {
				t.Fatalf("hookEventName = %q", got.HookSpecificOutput.HookEventName)
			}
			ctx := got.HookSpecificOutput.AdditionalContext
			for _, s := range tt.contains {
				if !strings.Contains(ctx, s) {
					t.Errorf("context missing %q: %s", s, ctx)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(ctx, s) {
					t.Errorf("context must not contain %q: %s", s, ctx)
				}
			}
		})
	}
}

// 末尾だけを読むとき、途中から始まった最初の行を捨てても今の batch は見つかる。
func TestTailRead(t *testing.T) {
	t.Parallel()
	filler := `{"type":"user","message":{"content":"` + strings.Repeat("a", tailBytes) + `"}}` + "\n"
	transcript := writeTranscript(t,
		filler,
		line(t, "msg_now", thinking("narration", "要約")),
		line(t, "msg_now", toolUse("toolu_a")),
	)
	var out bytes.Buffer
	run(bytes.NewReader(hookInput(t, "PostToolBatch", transcript, "", "toolu_a")), &out, noEnv)
	if !strings.Contains(out.String(), "要約") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestFeedbackLimitsSummaries(t *testing.T) {
	t.Parallel()
	var summaries []string
	for range maxSummaries + 2 {
		summaries = append(summaries, strings.Repeat("長", summaryRunes+10))
	}
	got := feedback(summaries)
	if !strings.Contains(got, "（7 件）") || !strings.Contains(got, "ほか 2 件") {
		t.Fatal(got)
	}
	if strings.Count(got, "\n- ") != maxSummaries+1 || !strings.Contains(got, strings.Repeat("長", summaryRunes)+"…") {
		t.Fatal(got)
	}
}
