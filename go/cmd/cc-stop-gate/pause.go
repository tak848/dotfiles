package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ツール呼び出しで turn を終え、その後の再開を外部が担う環境向けの設定。
// main agent の turn で最後に呼ばれたツールがこの一覧に含まれていれば、
// その Stop では判定をしない。Stop 入力には直前のツールが含まれないため、
// PostToolUse でマーカーを書き、Stop で消費する。
// PostToolUse で continue: false を返して turn を終える方式は使わない。ツール結果の直後に
// 締めの発話なしで turn が終わり、発話で終わる turn を前提にするホストでは turn が失敗しうるため。
const pauseToolsEnv = "CC_STOP_GATE_PAUSE_TOOLS"

// pauseMarkerCommand は PostToolUse / PostToolUseFailure / UserPromptSubmit に
// 登録するサブコマンド。引数で区別するのは、機能が無効なときに stdin
// （PostToolUse では tool_response 全体）を読まずに終えるため。
const pauseMarkerCommand = "pause-marker"

const maxSessionIDBytes = 128

type markerInput struct {
	Event     string `json:"hook_event_name"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	ToolName  string `json:"tool_name"`
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

// runMarker は、main agent で最後に成功したツールが一覧のツールかを session ごとに記録する。
// 読み書きの失敗は AI の作業では直せないので、無出力・exit 0 で終える。
func runMarker(stdin io.Reader, env lookupEnv) int {
	value, _ := env("CC_STOP_GATE")
	if disabled(value) {
		return 0
	}
	tools := pauseTools(env)
	if len(tools) == 0 {
		return 0
	}

	// tool_response は切り詰めで UTF-16 のサロゲートが割れていることがある。
	// 読めないとマーカーを消せず前の turn の値が残るので、置換して読む。
	var in markerInput
	if err := json.UnmarshalRead(stdin, &in, jsontext.AllowInvalidUTF8(true)); err != nil {
		return 0
	}
	// Stop は main agent の判定なので、subagent 内のツールでは書き換えない。
	if in.AgentID != "" {
		return 0
	}
	path, ok := markerPath(env, in.SessionID)
	if !ok {
		return 0
	}
	switch in.Event {
	case "PostToolUse":
		if tools[in.ToolName] {
			if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
				_ = os.WriteFile(path, []byte(in.ToolName+"\n"), 0o600)
			}
			return 0
		}
	case "PostToolUseFailure", "UserPromptSubmit":
	default:
		return 0
	}
	_ = os.Remove(path)
	return 0
}

// consumePause は、この session の turn が一覧のツールで終わっていれば、マーカーを消して true を返す。
// 消せたときだけ通す。存在しない以外の理由で消せない場合は存在を確かめられず、
// 通すと置き場の異常で全 session のゲートが黙って外れるので、通常の判定に進める。
func consumePause(env lookupEnv, sessionID string) bool {
	if len(pauseTools(env)) == 0 {
		return false
	}
	path, ok := markerPath(env, sessionID)
	return ok && os.Remove(path) == nil
}

func markerPath(env lookupEnv, sessionID string) (string, bool) {
	if !validSessionID(sessionID) {
		return "", false
	}
	var dir string
	if state, _ := env("XDG_STATE_HOME"); filepath.IsAbs(state) {
		dir = state
	} else if home, _ := env("HOME"); filepath.IsAbs(home) {
		dir = filepath.Join(home, ".local", "state")
	} else {
		return "", false
	}
	return filepath.Join(dir, "cc-stop-gate", "pause", sessionID), true
}

// session_id はファイル名になるので、区切り文字や相対指定を含むものは使わない。
func validSessionID(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > maxSessionIDBytes {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}
