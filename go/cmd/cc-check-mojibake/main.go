package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"strings"
)

type input struct {
	ToolName  string         `json:"tool_name"`
	ToolInput jsontext.Value `json:"tool_input"`
}

type writeInput struct {
	Content string `json:"content"`
}

type editInput struct {
	NewString string `json:"new_string"`
}

type multiEditInput struct {
	Edits []editInput `json:"edits"`
}

type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func main() {
	run(os.Stdin, os.Stdout, os.Stderr)
}

func run(stdin io.Reader, stdout, stderr io.Writer) {
	// 不正 UTF-8 も従来どおり U+FFFD として検出する。v2 の既定の拒否では
	// parse error で無出力になり、文字化けを含む書き込みを許してしまう。
	opts := json.JoinOptions(json.MatchCaseInsensitiveNames(true), jsontext.AllowInvalidUTF8(true))
	var in input
	if err := json.UnmarshalRead(stdin, &in, opts); err != nil {
		return
	}

	var found []string

	switch in.ToolName {
	case "Write":
		var ti writeInput
		if err := json.Unmarshal(in.ToolInput, &ti, opts); err != nil {
			return
		}
		if strings.ContainsRune(ti.Content, '\uFFFD') {
			found = append(found, "content")
		}
	case "Edit":
		var ti editInput
		if err := json.Unmarshal(in.ToolInput, &ti, opts); err != nil {
			return
		}
		if strings.ContainsRune(ti.NewString, '\uFFFD') {
			found = append(found, "new_string")
		}
	case "MultiEdit":
		var ti multiEditInput
		if err := json.Unmarshal(in.ToolInput, &ti, opts); err != nil {
			return
		}
		for i, e := range ti.Edits {
			if strings.ContainsRune(e.NewString, '\uFFFD') {
				found = append(found, fmt.Sprintf("edits[%d].new_string", i))
			}
		}
	default:
		return
	}

	if len(found) == 0 {
		return
	}

	for _, f := range found {
		fmt.Fprintf(stderr, "mojibake detected in field: %s\n", f)
	}

	out := hookOutput{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "deny",
			PermissionDecisionReason: fmt.Sprintf(
				"U+FFFD (文字化け) を検出しました。影響箇所を書き直してください。Fields: %s",
				strings.Join(found, ", "),
			),
		},
	}
	_ = json.MarshalEncode(jsontext.NewEncoder(stdout, jsontext.EscapeForHTML(true), jsontext.EscapeForJS(true)), out)
}
