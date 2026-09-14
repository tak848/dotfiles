package main

import (
	"regexp"
	"strings"
)

type signature uint8

const (
	unsigned signature = iota
	pledge
	survey
	waiting
)

type message struct {
	Signature signature
	Tells     bool
}

// 元のゲートと同じ日英の離脱パターン。意味的な完遂を証明するものではなく、
// 否定表現にも一致し得る。引用・コード・最終署名行は analyze 側で除く。
var tellsPattern = regexp.MustCompile(`(?i)(続けます|続けて|進めます|続行|続きは|着手し|次に進|次のフェーズ|次のステップ|次ステップ|次ターン|次のターン|別ターン|後続ターン|一旦|いったん|一度ここ|区切り|指示待ち|指示をお待ち|報告して指示|ここまでの報告|ここまでで一区切|残りは|残タスク|やり残|未実装|未対応|未着手|context ?残量|コンテキスト残量|次のセッション|別 ?PR(で|に|は|として|化)|別途対応|別途 ?PR|後続 ?PR|別チケット|別 ?issue|フォローアップ|今回はここまで|今回のスコープ|今回の ?PR では|今回は見送|スコープ外|対象外とし|次回対応|後日対応|MVP|i.ll continue|i will continue|let me continue|let.s continue|continuing (with|on|to|the|implementation|work|now)|next step|next phase|next turn|in the next (turn|step|session)|remaining (work|task|steps?|items?|implementation)|what.s (left|remaining)|left to (do|implement)|still (need|needs) to|yet to be (done|implemented|added)|i.ll pause|pausing here|stopping here|i.ll stop here|pick (this|it) up|to summarize (the )?progress|handing off|hand off|running low on context|low on context|context (window|budget|remaining|left)|shall i (continue|proceed)|should i (continue|proceed|keep)|would you like me to (continue|proceed)|awaiting (further )?(instructions?|guidance)|waiting for (your |further )?(instructions?|input|guidance)|for further instructions?|to be continued|i.ll resume|will resume|report back|separate pr|another pr|follow.?up|out of scope|not in scope|beyond .{0,4}scope|in a (later|separate|future|follow.?up)|will be (done|handled|addressed) (in|separately|later)|as a (separate|follow.?up)|defer(red|ring)?|leave .{0,20} for (later|now|a follow))`)

func analyze(text string) message {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	var result message
	var body strings.Builder
	var fenceChar byte
	var fenceLen int
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">") {
			continue
		}
		char, count := fence(trimmed)
		if fenceLen > 0 {
			if char == fenceChar && count >= fenceLen && strings.TrimSpace(trimmed[count:]) == "" {
				fenceLen = 0
			}
			continue
		}
		if count >= 3 {
			fenceChar, fenceLen = char, count
			continue
		}
		if i == last {
			result.Signature = parseSignature(line)
			if result.Signature != unsigned {
				continue
			}
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	result.Tells = tellsPattern.MatchString(body.String())
	return result
}

func fence(line string) (byte, int) {
	if len(line) == 0 || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	count := 0
	for count < len(line) && line[count] == line[0] {
		count++
	}
	return line[0], count
}

func parseSignature(line string) signature {
	for _, item := range []struct {
		word string
		kind signature
	}{
		{"完了誓約", pledge},
		{"調査完了", survey},
		{"作業待機", waiting},
	} {
		for _, colon := range []string{":", "："} {
			if summary, ok := strings.CutPrefix(line, item.word+colon); ok && strings.TrimSpace(summary) != "" {
				return item.kind
			}
		}
	}
	return unsigned
}
