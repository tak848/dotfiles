package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"strings"
)

// ツール呼び出しで turn を終え、その後の再開を外部が担う環境向けの設定。
// Stop は matcher も hook の if も効かず、入力に直前のツールも含まれないので、
// Stop 側では見分けられない。代わりに一覧のツールの PostToolUse で
// continue: false を返し、次のモデル呼び出しの前に turn を終える。
// この終わり方では Stop hook が発火しないので、Stop ゲートは判定しない。
const pauseToolsEnv = "CC_STOP_GATE_PAUSE_TOOLS"

// pauseRejectKeysEnv は、一覧のツールが成功として返した結果の中身で「断られた」を見分けるキー名の一覧。
// 断られたら直して呼び直すツールでは、turn を終えるとモデルが直せない。列挙したキーのどれかが
// false の結果では止めない。ツールの呼び出し自体の失敗（PostToolUseFailure）は、もともと止めない。
const pauseRejectKeysEnv = "CC_STOP_GATE_PAUSE_REJECT_KEYS"

// pauseToolCommand は PostToolUse（全ツール）に登録するサブコマンド。引数で区別するのは、
// 機能が無効なときに stdin（tool_response 全体）を読まずに終えるため。
const pauseToolCommand = "pause-tool"

type pauseInput struct {
	Event        string         `json:"hook_event_name"`
	AgentID      string         `json:"agent_id"`
	ToolName     string         `json:"tool_name"`
	ToolResponse jsontext.Value `json:"tool_response"`
}

// stopReason を付けるとユーザーに表示され、会話にも残るので付けない。
type pauseOutput struct {
	Continue bool `json:"continue"`
}

func pauseTools(env lookupEnv) map[string]bool {
	return nameSet(env, pauseToolsEnv)
}

// nameSet は、カンマ区切りの一覧を前後の空白を除いた完全一致の集合にする。
func nameSet(env lookupEnv, key string) map[string]bool {
	value, _ := env(key)
	names := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names[name] = true
		}
	}
	return names
}

// rejected は、tool_response が列挙したキーのどれかを JSON の false にしているかを返す。
// 見るのはトップレベルのキーだけ。読めない・形が違う・キーが無いときは false を返し、今までどおり止める。
// PostToolUse の tool_response の形は tool による（公式 docs hooks の PostToolUse input）。
// MCP のツールでは、structuredContent が無ければ content block の配列、あれば structuredContent を
// JSON にした文字列で届く（Claude Code 2.1.318 で観測）。組み込みのツールは object。
func rejected(response jsontext.Value, keys map[string]bool) bool {
	if len(keys) == 0 {
		return false
	}
	switch response.Kind() {
	case '{':
		return hasFalseKey(response, keys)
	case '"':
		var text string
		if json.Unmarshal(response, &text, jsontext.AllowInvalidUTF8(true)) != nil {
			return false
		}
		return hasFalseKey(jsontext.Value(text), keys)
	case '[':
		// 最初の text block の本文を JSON として読む。
		var blocks []struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if json.Unmarshal(response, &blocks, jsontext.AllowInvalidUTF8(true)) != nil {
			return false
		}
		for _, block := range blocks {
			if block.Type == "text" && block.Text != nil {
				return hasFalseKey(jsontext.Value(*block.Text), keys)
			}
		}
	}
	return false
}

func hasFalseKey(raw jsontext.Value, keys map[string]bool) bool {
	var object map[string]jsontext.Value
	if json.Unmarshal(raw, &object, jsontext.AllowInvalidUTF8(true)) != nil {
		return false
	}
	for key, value := range object {
		if keys[key] && value.Kind() == 'f' {
			return true
		}
	}
	return false
}

// runPause は、main agent が一覧のツールを呼んだときだけ continue: false を返す。
// 入出力の異常は AI の作業では直せないので、無出力・exit 0 で終える。
func runPause(stdin io.Reader, stdout io.Writer, env lookupEnv) int {
	value, _ := env("CC_STOP_GATE")
	if disabled(value) {
		return 0
	}
	tools := pauseTools(env)
	if len(tools) == 0 {
		return 0
	}

	// tool_response は切り詰めで UTF-16 のサロゲートが割れていることがあるので、置換して読む。
	var in pauseInput
	if err := json.UnmarshalRead(stdin, &in, jsontext.AllowInvalidUTF8(true)); err != nil {
		return 0
	}
	// Stop は main agent の判定なので、subagent 内のツールでは止めない。
	if in.Event != "PostToolUse" || in.AgentID != "" || !tools[in.ToolName] {
		return 0
	}
	// 断られた結果ではモデルが直して呼び直せるよう、turn を続ける。
	if rejected(in.ToolResponse, nameSet(env, pauseRejectKeysEnv)) {
		return 0
	}
	_ = json.MarshalWrite(stdout, pauseOutput{Continue: false})
	return 0
}
