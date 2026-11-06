package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tak848/dotfiles/go/internal/gitstate"
)

func envMap(values map[string]string) lookupEnv {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		mode   string
		text   string
		tasks  string
		crons  string
		env    map[string]string
		issues []string
		// active は Stop 入力の stop_hook_active（Claude Code が既に Stop hook による継続中か）。
		active   bool
		blocked  bool
		checked  bool
		contains string
		excludes string
	}{
		"normal complete":                {mode: "default", text: "完了誓約: 全項目を検証", checked: true},
		"no plan file":                   {mode: "acceptEdits", text: "完了誓約: 依頼を完了", checked: true},
		"auto":                           {mode: "auto", text: "調査完了: 仕様を確認", checked: true},
		"no signature":                   {mode: "default", text: "終わりました。", blocked: true, checked: true, contains: "plan があるなら"},
		"empty text":                     {mode: "default", blocked: true, checked: true, contains: "全項目"},
		"tells":                          {mode: "default", text: "続けます", blocked: true, checked: true, contains: "表現に一致しました"},
		"signed tells":                   {mode: "default", text: "次回対応します\n完了誓約: 完了", blocked: true, checked: true, contains: "終了できません"},
		"dirty no edit tool":             {mode: "default", text: "完了誓約: 完了", issues: []string{"未 commit"}, blocked: true, checked: true, contains: "未 commit"},
		"unknown state":                  {mode: "default", text: "完了誓約: 完了", issues: []string{gitstate.CheckFailurePrefix + " [github-repository] 認証を確認できない"}, checked: true},
		"plan no signature":              {mode: "plan", text: "計画を示しました。", blocked: true, contains: "ExitPlanMode"},
		"plan pledge":                    {mode: "plan", text: "完了誓約: 完了", blocked: true, contains: "完了誓約は無効"},
		"plan survey":                    {mode: "plan", text: "調査完了: 調査結果を回答"},
		"plan survey tells":              {mode: "plan", text: "続きは次回\n調査完了: 完了", blocked: true, contains: "終了できません"},
		"waiting agent":                  {mode: "default", text: "作業待機: agent の完了", tasks: `[{"id":"agent-1","type":"subagent","status":"running"}]`},
		"waiting cron":                   {mode: "default", text: "作業待機: 予定の通知", crons: `[{"id":"cron-1"}]`},
		"plan waiting":                   {mode: "plan", text: "作業待機: 調査 agent", tasks: `[{"id":"agent-1"}]`},
		"fake waiting":                   {mode: "default", text: "作業待機: 自分の作業", blocked: true, contains: "待つ対象が無い"},
		"empty task":                     {mode: "default", text: "作業待機: 作業", tasks: `[null,{},"x"]`, blocked: true, contains: "待つ対象が無い"},
		"background alone not exemption": {mode: "default", text: "終了します", tasks: `[{"id":"monitor-1"}]`, blocked: true, checked: true},
		// Stop hook による継続中でも、作業待機以外の判定は解除しない。
		"no signature while continuing":      {mode: "default", text: "終わりました。", active: true, blocked: true, checked: true, contains: "plan があるなら"},
		"dirty while continuing":             {mode: "default", text: "完了誓約: 完了", issues: []string{"未 commit"}, active: true, blocked: true, checked: true, contains: "未 commit"},
		"plan no signature while continuing": {mode: "plan", text: "計画を示しました。", active: true, blocked: true, contains: "ExitPlanMode"},
		"fake waiting while continuing":      {mode: "default", text: "作業待機: 自分の作業", active: true, blocked: true, contains: "待つ対象が無い"},
		// ホストが turn の外の出来事を新しい turn として届ける環境の待ち（CC_STOP_GATE_EXTERNAL_WAIT）。
		// 無効なときは、Stop hook による継続中でも今と同じく差し戻す。
		"external wait unset":                  {mode: "default", text: "作業待機: 任せた作業の結果", blocked: true, contains: "待つ対象が無い"},
		"external wait unset while continuing": {mode: "default", text: "作業待機: 任せた作業の結果", active: true, blocked: true, contains: "待つ対象が無い", excludes: "作業待機の前に確かめろ"},
		"external wait not enabled":            {mode: "default", text: "作業待機: 任せた作業の結果", env: map[string]string{externalWaitEnv: "anything"}, active: true, blocked: true, contains: "待つ対象が無い"},
		"plan external wait unset":             {mode: "plan", text: "作業待機: 任せた調査の結果", active: true, blocked: true, contains: "待つ対象が無い"},
		// 有効なときは、1 回目を確認の文で差し戻し、書き直した作業待機（stop_hook_active が true）を通す。
		"external wait first":               {mode: "default", text: "作業待機: 任せた作業の結果", env: map[string]string{externalWaitEnv: "1"}, blocked: true, contains: "【作業待機の前に確かめろ】", excludes: "待つ対象が無い"},
		"external wait rewritten":           {mode: "default", text: "作業待機: 任せた作業の結果", env: map[string]string{externalWaitEnv: "1"}, active: true},
		"external wait inflight":            {mode: "default", text: "作業待機: 任せた作業の結果", env: map[string]string{externalWaitEnv: "1"}, tasks: `[{"id":"agent-1"}]`},
		"external wait first skips git":     {mode: "acceptEdits", text: "作業待機: 承認の結果", env: map[string]string{externalWaitEnv: " TRUE "}, issues: []string{"未 commit"}, blocked: true, contains: "本当に必須か", excludes: "未 commit"},
		"external wait rewritten skips git": {mode: "acceptEdits", text: "作業待機: 承認の結果", env: map[string]string{externalWaitEnv: " TRUE "}, issues: []string{"未 commit"}, active: true},
		"external wait plan first":          {mode: "plan", text: "作業待機: 任せた調査の結果", env: map[string]string{externalWaitEnv: "yes"}, blocked: true, contains: "【作業待機の前に確かめろ】", excludes: "ExitPlanMode"},
		"external wait plan rewritten":      {mode: "plan", text: "作業待機: 任せた調査の結果", env: map[string]string{externalWaitEnv: "yes"}, active: true},
		"external wait empty target":        {mode: "default", text: "作業待機: ", env: map[string]string{externalWaitEnv: "on"}, blocked: true, checked: true, contains: "plan があるなら", excludes: "待つ対象が無い"},
		"external wait empty target active": {mode: "default", text: "作業待機: ", env: map[string]string{externalWaitEnv: "on"}, active: true, blocked: true, checked: true, contains: "plan があるなら", excludes: "作業待機の前に確かめろ"},
		"external wait signed tells first":  {mode: "default", text: "続きは次回\n作業待機: 任せた作業の結果", env: map[string]string{externalWaitEnv: "1"}, blocked: true, contains: "【作業待機の前に確かめろ】", excludes: "表現に一致しました"},
		"external wait signed tells":        {mode: "default", text: "続きは次回\n作業待機: 任せた作業の結果", env: map[string]string{externalWaitEnv: "1"}, active: true},
		"external wait other signature":     {mode: "default", text: "終わりました。", env: map[string]string{externalWaitEnv: "1"}, active: true, blocked: true, checked: true, contains: "plan があるなら"},
		"question":                          {mode: "default", text: "| 案 | 影響 |\n|---|---|\n| A | 小 |\n質問予告: A と B のどちらにするか", blocked: true, contains: "今すぐ AskUserQuestion を呼べ"},
		"question skips tells":              {mode: "default", text: "残りは B 案の判断だけです。\n質問予告: B 案にするか", blocked: true, contains: "質問予告を受け付けた"},
		"question skips git issues":         {mode: "acceptEdits", text: "質問予告: どちらにするか", issues: []string{"未 commit"}, blocked: true, contains: "判断材料を繰り返すな"},
		"plan question":                     {mode: "plan", text: "質問予告: 方針を選んでもらう", blocked: true, contains: "今すぐ AskUserQuestion を呼べ"},
		"question not final":                {mode: "default", text: "質問予告: どちらにするか\n補足", blocked: true, checked: true, contains: "質問予告: <問いの要約>"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			obj := map[string]any{
				"hook_event_name": "Stop", "permission_mode": tt.mode,
				"cwd": "/example/repo", "last_assistant_message": tt.text,
				"stop_hook_active": tt.active,
				// これは参照しない。ファイルが無くても正しく動く。
				"transcript_path": "/not/a/transcript",
			}
			if tt.tasks != "" {
				obj["background_tasks"] = jsontext.Value(tt.tasks)
			}
			if tt.crons != "" {
				obj["session_crons"] = jsontext.Value(tt.crons)
			}
			data, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			checked := false
			check := func(ctx context.Context, cwd string, owners []string) []string {
				checked = true
				if cwd != "/example/repo" || !reflect.DeepEqual(owners, []string{"tak848"}) {
					t.Fatalf("unexpected args: %s %v", cwd, owners)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("check has no deadline")
				}
				return tt.issues
			}
			var out bytes.Buffer
			if code := run(bytes.NewReader(data), &out, envMap(tt.env), check); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			if checked != tt.checked {
				t.Errorf("checked = %v, want %v", checked, tt.checked)
			}
			if !tt.blocked {
				if out.Len() != 0 {
					t.Fatalf("unexpected output: %s", out.String())
				}
				return
			}
			var got decision
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Decision != "block" || got.Reason == "" {
				t.Fatalf("not blocked: %#v", got)
			}
			if !strings.Contains(got.Reason, tt.contains) {
				t.Errorf("reason missing %q: %s", tt.contains, got.Reason)
			}
			if tt.excludes != "" && strings.Contains(got.Reason, tt.excludes) {
				t.Errorf("reason must not contain %q: %s", tt.excludes, got.Reason)
			}
		})
	}
}

// Stop 入力の stop_hook_active を facts に渡す。外からの待ちの作業待機は、値が true のときだけ通り、
// false と、キーが無いとき（Stop hook による継続中ではない）は確認の文で差し戻す。
func TestStopHookActiveInput(t *testing.T) {
	t.Parallel()
	env := envMap(map[string]string{externalWaitEnv: "1"})
	for name, tt := range map[string]struct {
		field   string
		blocked bool
	}{
		"true":    {`,"stop_hook_active":true`, false},
		"false":   {`,"stop_hook_active":false`, true},
		"missing": {"", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			payload := `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/repo","last_assistant_message":"作業待機: 任せた作業の結果","background_tasks":[],"session_crons":[]` + tt.field + `}`
			var out bytes.Buffer
			check := func(context.Context, string, []string) []string { t.Fatal("waiting must not inspect git"); return nil }
			if code := run(strings.NewReader(payload), &out, env, check); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			if !tt.blocked {
				if out.Len() != 0 {
					t.Fatalf("unexpected output: %s", out.String())
				}
				return
			}
			var got decision
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got != block(waitCheck) {
				t.Fatalf("got %v, want waitCheck", got)
			}
		})
	}
}

func TestEnabled(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"1", "true", "TRUE", " on ", "Yes"} {
		if !enabled(value) {
			t.Errorf("enabled(%q) = false", value)
		}
	}
	for _, value := range []string{"", " ", "0", "false", "off", "no", "anything", "2"} {
		if enabled(value) {
			t.Errorf("enabled(%q) = true", value)
		}
	}
}

func TestInvalidInput(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"empty": "", "malformed": "{", "null": "null", "array": "[]", "missing": "{}",
		"two objects": "{}{}", "oversized": strings.Repeat(" ", maxInputBytes+1),
		"missing message": `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/"}`,
		"missing mode":    `{"hook_event_name":"Stop","last_assistant_message":"完了誓約: 終了"}`,
		"unknown mode":    `{"hook_event_name":"Stop","permission_mode":"typo","last_assistant_message":"完了誓約: 終了"}`,
		"null message":    `{"hook_event_name":"Stop","permission_mode":"default","last_assistant_message":null}`,
		"wrong event":     `{"hook_event_name":"PostToolUse","last_assistant_message":"完了誓約: 終了"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			check := func(context.Context, string, []string) []string { t.Fatal("must not inspect git"); return nil }
			if code := run(strings.NewReader(text), &out, envMap(nil), check); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			// AI の作業では直せない入力の異常なので、差し戻さずに通す。
			if out.Len() != 0 {
				t.Fatalf("output = %s", &out)
			}
		})
	}
}

func TestMissingCWDStillChecksMessage(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		text    string
		blocked bool
	}{{"完了誓約: 全項目を確認", false}, {"報告のみ", true}} {
		payload, err := json.Marshal(map[string]string{"hook_event_name": "Stop", "permission_mode": "default", "last_assistant_message": tt.text})
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if code := run(bytes.NewReader(payload), &out, envMap(nil), nil); code != 0 {
			t.Fatal(code)
		}
		if (out.Len() != 0) != tt.blocked || strings.Contains(out.String(), "ディレクトリ") {
			t.Fatal(out.String())
		}
	}
}

func TestV2RejectsDuplicateMembers(t *testing.T) {
	t.Parallel()
	data := `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/repo","last_assistant_message":"続けます","last_assistant_message":"完了誓約: 完了"}`
	var out bytes.Buffer
	check := func(context.Context, string, []string) []string {
		t.Fatal("duplicate JSON must be rejected before git checks")
		return nil
	}
	if code := run(strings.NewReader(data), &out, envMap(nil), check); code != 0 || out.Len() != 0 {
		t.Fatalf("exit = %d, output = %q", code, out.String())
	}
}

func TestOptOut(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0", "false", "OFF", " no "} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if code := run(failingReader{}, &out, envMap(map[string]string{"CC_STOP_GATE": value}), nil); code != 0 || out.Len() != 0 {
				t.Fatalf("exit = %d, output = %q", code, &out)
			}
		})
	}
	for _, value := range []string{"", "1", "true", "anything"} {
		if disabled(value) {
			t.Errorf("unexpected opt-out: %q", value)
		}
	}
}

func TestPROwners(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		env  map[string]string
		want []string
	}{
		"default":    {nil, []string{"tak848"}},
		"empty":      {map[string]string{"CC_STOP_GATE_REQUIRE_PR_OWNERS": ""}, nil},
		"whitespace": {map[string]string{"CC_STOP_GATE_REQUIRE_PR_OWNERS": " , "}, nil},
		"override":   {map[string]string{"CC_STOP_GATE_REQUIRE_PR_OWNERS": " ExampleOrg, example-user,EXAMPLE-USER "}, []string{"exampleorg", "example-user"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := requiredPROwners(envMap(tt.env)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("owners = %v, want %v", got, tt.want)
			}
		})
	}
	var out bytes.Buffer
	called := false
	data := `{"hook_event_name":"Stop","permission_mode":"default","cwd":"/repo","last_assistant_message":"完了誓約: 完了"}`
	run(strings.NewReader(data), &out, envMap(map[string]string{"CC_STOP_GATE_REQUIRE_PR_OWNERS": ""}), func(_ context.Context, _ string, owners []string) []string {
		called = true
		if len(owners) != 0 {
			t.Errorf("owners = %v", owners)
		}
		return []string{"未 push"}
	})
	if !called || !strings.Contains(out.String(), "未 push") {
		t.Fatal("empty owners must not disable other checks")
	}
}

func TestBoundedReasons(t *testing.T) {
	t.Parallel()
	got := decide(facts{Message: message{}, Issues: []string{strings.Repeat("長い理由", 10000) + "\x1b[31m\x00"}})
	if len(got.Reason) > maxReasonBytes || !utf8.ValidString(got.Reason) {
		t.Fatal("invalid bounded reason")
	}
	if !strings.Contains(got.Reason, "plan があるなら") || !strings.Contains(got.Reason, "全項目") {
		t.Fatal("core completion instructions truncated")
	}
	if strings.ContainsAny(got.Reason, "\x1b\x00") {
		t.Fatal("control characters leaked")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failure") }

func TestIOErrors(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if code := run(failingReader{}, &out, envMap(nil), nil); code != 0 || out.Len() != 0 {
		t.Fatal("read failure must pass without output")
	}
	// Stop の exit 2 は block になり stderr が AI に渡るので、出力失敗でも 0 で終える。
	payload := `{"hook_event_name":"Stop","permission_mode":"plan","last_assistant_message":"報告のみ"}`
	if code := run(strings.NewReader(payload), failingWriter{}, envMap(nil), nil); code != 0 {
		t.Fatalf("write failure exit = %d", code)
	}
	if code := emit(io.Discard, decision{}); code != 0 {
		t.Fatalf("allow exit = %d", code)
	}
}
