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

// pauseToolCommand は PostToolUse（全ツール）に登録するサブコマンド。引数で区別するのは、
// 機能が無効なときに stdin（tool_response 全体）を読まずに終えるため。
const pauseToolCommand = "pause-tool"

type pauseInput struct {
	Event    string `json:"hook_event_name"`
	AgentID  string `json:"agent_id"`
	ToolName string `json:"tool_name"`
}

// stopReason を付けるとユーザーに表示され、会話にも残るので付けない。
type pauseOutput struct {
	Continue bool `json:"continue"`
}

func pauseTools(env lookupEnv) map[string]bool {
	value, _ := env(pauseToolsEnv)
	tools := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			tools[name] = true
		}
	}
	return tools
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
	_ = json.MarshalWrite(stdout, pauseOutput{Continue: false})
	return 0
}
