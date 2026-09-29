// cc-narration-check は PostToolBatch hook。直前の tool 呼び出しの前に書いた text が
// サーバー側で要約され、ユーザーに原文が届かなかったことを検出してモデルに知らせる。
//
// Opus 5.5 / Fable 5 / Fable 5.1 などでは、tool result の後・次の tool 呼び出しの前に
// 書いた text（progress update）が thinking ブロックとして返り、原文は画面にも transcript
// にも残らない（anthropics/claude-code#74558、公式 docs の "Progress updates between tool
// calls"）。モデル側の context には原文が残るので、モデルは伝えたつもりでいる。
//
// 要約されたブロックは、signature を base64 デコードした先頭付近に "narration" という
// 種別が入る（通常の推論は "thinking"）。形（thinking が 2 つ並ぶ等）で推測するより誤検知が
// 無い。この判別方法は #74558 の aultra 氏のコメントによる。検出したら additionalContext で
// 「必要ならターン最後の text で書き直せ」と伝える。ターン最後の text は要約されない。
// PostToolBatch で検出して書き直させる構成は podlayer/message-drop-sentinel（MIT）を参考に
// したが、コードは流用していない。
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxInputBytes = 64 << 20
	// 今の batch の tool_use を持つ assistant 行は transcript の末尾付近にある。
	// 大きい tool result が挟まっても届くよう、余裕を持って末尾だけを読む。
	tailBytes = 16 << 20
	// signature の先頭だけをデコードする。種別の語は先頭 100 byte 以内に入る。
	signaturePrefix = 160
	summaryRunes    = 200
	maxSummaries    = 5
)

type input struct {
	Event          string `json:"hook_event_name"`
	AgentID        string `json:"agent_id"`
	TranscriptPath string `json:"transcript_path"`
	ToolCalls      []struct {
		ToolUseID string `json:"tool_use_id"`
	} `json:"tool_calls"`
}

type output struct {
	HookSpecificOutput hookOutput `json:"hookSpecificOutput"`
}

type hookOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type record struct {
	Type    string `json:"type"`
	Message struct {
		ID      string  `json:"id"`
		Content []block `json:"content"`
	} `json:"message"`
}

type block struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type lookupEnv func(string) (string, bool)

func main() {
	run(os.Stdin, os.Stdout, os.LookupEnv)
}

// run は検出できたときだけ出力する。入力や transcript の異常はモデルの作業では直せないので、
// 無出力・exit 0 で通常の処理へ戻す。
func run(stdin io.Reader, stdout io.Writer, env lookupEnv) int {
	value, _ := env("CC_NARRATION_CHECK")
	if disabled(value) {
		return 0
	}
	data, err := io.ReadAll(io.LimitReader(stdin, maxInputBytes+1))
	if err != nil || len(data) > maxInputBytes {
		return 0
	}
	// tool_response は切り詰めで UTF-16 のサロゲートが割れていることがあるので、置換して読む。
	var in input
	if err := json.Unmarshal(data, &in, jsontext.AllowInvalidUTF8(true)); err != nil {
		return 0
	}
	// subagent の途中の text はもともとユーザーに見せる場所ではない。
	if in.Event != "PostToolBatch" || in.AgentID != "" || !filepath.IsAbs(in.TranscriptPath) {
		return 0
	}
	ids := map[string]bool{}
	for _, call := range in.ToolCalls {
		if call.ToolUseID != "" {
			ids[call.ToolUseID] = true
		}
	}
	if len(ids) == 0 {
		return 0
	}
	f, err := os.Open(in.TranscriptPath)
	if err != nil {
		return 0
	}
	defer f.Close()
	summaries, ok := narrations(f, ids)
	if !ok || len(summaries) == 0 {
		return 0
	}
	_ = json.MarshalWrite(stdout, output{HookSpecificOutput: hookOutput{
		HookEventName:     "PostToolBatch",
		AdditionalContext: feedback(summaries),
	}})
	return 0
}

// narrations は、batch の tool_use を含む assistant メッセージから、要約に置き換わった
// ブロックの表示文（display が omitted なら空文字）を返す。transcript は非同期に書かれるので、
// 該当メッセージがまだ無ければ何も返さない（取りこぼしは許容する）。
func narrations(f *os.File, ids map[string]bool) ([]string, bool) {
	info, err := f.Stat()
	if err != nil {
		return nil, false
	}
	offset := max(info.Size()-tailBytes, 0)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, false
	}
	reader := bufio.NewReaderSize(f, 1<<20)
	if offset > 0 {
		// 途中から読み始めた最初の行は壊れているので捨てる。
		if _, err := reader.ReadBytes('\n'); err != nil {
			return nil, false
		}
	}

	type found struct {
		matched   bool
		summaries []string
	}
	messages := map[string]*found{}
	var order []string
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 && bytes.Contains(line, []byte(`"assistant"`)) {
			var r record
			if json.Unmarshal(bytes.TrimSpace(line), &r) == nil && r.Type == "assistant" && r.Message.ID != "" {
				m := messages[r.Message.ID]
				if m == nil {
					m = &found{}
					messages[r.Message.ID] = m
					order = append(order, r.Message.ID)
				}
				for _, b := range r.Message.Content {
					switch {
					case b.Type == "tool_use" && ids[b.ID]:
						m.matched = true
					case b.Type == "thinking" && signatureKind(b.Signature) == "narration":
						m.summaries = append(m.summaries, b.Thinking)
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false
		}
	}
	var result []string
	for _, id := range order {
		if m := messages[id]; m.matched {
			result = append(result, m.summaries...)
		}
	}
	return result, true
}

// signatureKind は signature の先頭をデコードして "narration" / "thinking" / "" を返す。
// signature の形式は公開されていないので、読めなければ判定しない。
func signatureKind(signature string) string {
	n := min(len(signature), signaturePrefix)
	n -= n % 4
	raw, err := base64.StdEncoding.DecodeString(signature[:n])
	if err != nil {
		return ""
	}
	narration := bytes.Index(raw, []byte("narration"))
	thinking := bytes.Index(raw, []byte("thinking"))
	switch {
	case narration >= 0 && (thinking < 0 || narration < thinking):
		return "narration"
	case thinking >= 0:
		return "thinking"
	default:
		return ""
	}
}

func feedback(summaries []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【途中の text がユーザーに届いていない】直前の tool 呼び出しの前に書いた text（%d 件）は、サーバー側で要約されて thinking に置き換わった（anthropics/claude-code#74558）。原文はユーザーの画面にも transcript にも無い。", len(summaries))
	var shown []string
	for _, s := range summaries {
		if s = oneLine(s); s != "" {
			shown = append(shown, s)
		}
	}
	if len(shown) == 0 {
		b.WriteString("画面には何も表示されていない。")
	} else {
		b.WriteString("画面に出たのは次の要約だけ:")
		for i, s := range shown {
			if i == maxSummaries {
				fmt.Fprintf(&b, "\n- ほか %d 件", len(shown)-maxSummaries)
				break
			}
			b.WriteString("\n- " + s)
		}
	}
	b.WriteString("\nユーザーが読む必要のある内容（調査結果・判断の根拠・質問の材料）なら、このターン最後の text に改めて書け。tool 呼び出しの前に書き直しても同じように消える。単なる進捗報告なら書き直さなくてよい。")
	return b.String()
}

// oneLine は要約を 1 行に畳み、長さを制限する。要約はサーバーが書いた文なので制御文字を落とす。
func oneLine(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t'
	}), " ")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, "?"))
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > summaryRunes {
		s = string([]rune(s)[:summaryRunes]) + "…"
	}
	return s
}

func disabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "off", "no":
		return true
	default:
		return false
	}
}
