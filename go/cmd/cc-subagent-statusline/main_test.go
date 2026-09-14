package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/tak848/dotfiles/go/internal/render"
)

func decodeRows(t *testing.T, out string) []row {
	t.Helper()

	var rows []row
	for line := range strings.SplitSeq(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		var r row
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("output line is not valid JSON (Claude Code would warn): %q: %v", line, err)
		}
		rows = append(rows, r)
	}
	return rows
}

func TestRenderTask(t *testing.T) {
	t.Parallel()

	base := task{
		ID:                "t1",
		Name:              "Explore",
		Type:              "general-purpose",
		Status:            "running",
		Description:       "検索中",
		Label:             "検索中",
		Model:             "claude-sonnet-4-5-20250929",
		Effort:            jsontext.Value(`"high"`),
		ContextWindowSize: 200_000,
		TokenCount:        24_000,
	}

	tests := map[string]struct {
		mutate      func(*task)
		width       int
		wantContain []string
		wantAbsent  []string
	}{
		"full": {
			mutate:      func(*task) {},
			width:       80,
			wantContain: []string{"sonnet·high", "12%", "Explore", "検索中"},
		},
		// effort が数値のトークン予算で来ることがある。桁が大きいので出さない。
		"numeric_effort": {
			mutate:      func(tk *task) { tk.Effort = jsontext.Value(`32000`) },
			width:       80,
			wantContain: []string{"sonnet"},
			wantAbsent:  []string{"32000"},
		},
		"missing_effort": {
			mutate:      func(tk *task) { tk.Effort = nil },
			width:       80,
			wantContain: []string{"sonnet"},
		},
		"missing_model": {
			mutate:      func(tk *task) { tk.Model = "" },
			width:       80,
			wantContain: []string{"Explore"},
		},
		// contextWindowSize が 0 でもゼロ除算せず、枠だけ残して伏せる。
		"zero_context_window": {
			mutate:      func(tk *task) { tk.ContextWindowSize = 0 },
			width:       80,
			wantContain: []string{"Explore", "--%"},
		},
		"zero_tokens": {
			mutate:      func(tk *task) { tk.TokenCount = 0 },
			width:       80,
			wantContain: []string{"Explore", "--%"},
		},
		"no_name": {
			mutate:      func(tk *task) { tk.Name = "" },
			width:       80,
			wantContain: []string{"general-purpose"},
		},
		"no_name_no_type": {
			mutate:      func(tk *task) { tk.Name, tk.Type = "", "" },
			width:       80,
			wantContain: []string{"task"},
		},
		// label が無いときだけ description に落ちる。
		"label_falls_back": {
			mutate:      func(tk *task) { tk.Label = "" },
			width:       80,
			wantContain: []string{"検索中"},
		},
		"narrow": {
			mutate:      func(*task) {},
			width:       12,
			wantContain: []string{"Explore"},
		},
		"unlimited_width": {
			mutate:      func(*task) {},
			width:       0,
			wantContain: []string{"Explore", "検索中"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tk := base
			tt.mutate(&tk)
			got := renderTask(tk, tt.width)

			if got == "" {
				t.Fatal("content must not be empty; an empty content hides the row")
			}
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("content contains a newline: %q", got)
			}
			if tt.width > 0 && render.Width(got) > tt.width {
				t.Errorf("content width = %d, exceeds %d: %q", render.Width(got), tt.width, got)
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("renderTask() = %q, want it to contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("renderTask() = %q, want it not to contain %q", got, absent)
				}
			}
		})
	}
}

func TestRenderTaskSanitizesHostileStrings(t *testing.T) {
	t.Parallel()

	tk := task{
		ID:          "t1",
		Name:        "evil\nsecond row",
		Description: "desc\033[31m",
		Status:      "running",
	}
	got := renderTask(tk, 80)
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("content contains a newline: %q", got)
	}
}

func TestRenderRows(t *testing.T) {
	t.Parallel()

	in := input{
		Columns: 80,
		Tasks: []task{
			{ID: "t1", Name: "Explore", Status: "running"},
			{ID: "t2", Name: "Plan", Status: "completed"},
		},
	}

	rows := decodeRows(t, renderRows(in))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, want := range []string{"t1", "t2"} {
		if rows[i].ID != want {
			t.Errorf("rows[%d].ID = %q, want %q", i, rows[i].ID, want)
		}
		if rows[i].Content == "" {
			t.Errorf("rows[%d].Content is empty, which would hide the row", i)
		}
	}
}

func TestRenderRowsEdgeCases(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in       input
		wantRows int
	}{
		"empty":     {in: input{Columns: 80}, wantRows: 0},
		"no_id":     {in: input{Columns: 80, Tasks: []task{{Name: "Explore"}}}, wantRows: 0},
		"zero_cols": {in: input{Columns: 0, Tasks: []task{{ID: "t1", Name: "Explore"}}}, wantRows: 1},
		"tiny_cols": {in: input{Columns: 2, Tasks: []task{{ID: "t1", Name: "Explore"}}}, wantRows: 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rows := decodeRows(t, renderRows(tt.in))
			if len(rows) != tt.wantRows {
				t.Errorf("got %d rows, want %d", len(rows), tt.wantRows)
			}
		})
	}
}

// TestDecodeMixedEffortTypes は 1 タスクの effort が数値でも、他のタスクの
// 描画が巻き添えで失われないことを確かめる。型を固定して受けるとここで
// decode 全体が落ち、パネルが丸ごと既定描画に戻る。
func TestDecodeMixedEffortTypes(t *testing.T) {
	t.Parallel()

	const raw = `{"columns": 80, "tasks": [
		{"id": "t1", "name": "A", "effort": "high"},
		{"id": "t2", "name": "B", "effort": 32000},
		{"id": "t3", "name": "C"}
	]}`

	var in input
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(in.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(in.Tasks))
	}
	if rows := decodeRows(t, renderRows(in)); len(rows) != 3 {
		t.Errorf("got %d rows, want 3", len(rows))
	}
}

func TestRenderRowsJSONLFramingAndEscaping(t *testing.T) {
	t.Parallel()
	id := "<id>&\n" + string([]rune{0x2028, 0x2029})
	in := input{Tasks: []task{{ID: id, Name: "<A>&"}, {ID: "second", Name: "B"}}}
	out := renderRows(in)
	if strings.Count(out, "\n") != 2 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("want exactly two newline-terminated JSON lines, got %q", out)
	}
	for _, escaped := range []string{"\\u003c", "\\u003e", "\\u0026", "\\u2028", "\\u2029", "\\n"} {
		if !strings.Contains(out, escaped) {
			t.Errorf("missing legacy JSON escaping %q in %q", escaped, out)
		}
	}
	rows := decodeRows(t, out)
	if len(rows) != 2 || rows[0].ID != id || rows[1].ID != "second" {
		t.Fatalf("row IDs did not round-trip: %+v", rows)
	}
	if !strings.Contains(rows[0].Content, "<A>&") {
		t.Errorf("escaped content did not round-trip: %q", rows[0].Content)
	}
}

func TestDecodeNullAndEmptyTasks(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{}`, `{"tasks":null}`, `{"tasks":[]}`} {
		var in input
		if err := json.UnmarshalRead(strings.NewReader(raw), &in, json.MatchCaseInsensitiveNames(true)); err != nil {
			t.Fatal(err)
		}
		if got := renderRows(in); got != "" {
			t.Errorf("%s produced %q, want no JSONL rows", raw, got)
		}
	}
}

func TestDecodeCaseAndNullEffort(t *testing.T) {
	t.Parallel()
	const raw = `{"COLUMNS":80,"TASKS":[{"ID":"t1","NAME":"A","EFFORT":null,"TOKENCOUNT":null,"CONTEXTWINDOWSIZE":null}]}`
	var in input
	if err := json.UnmarshalRead(strings.NewReader(raw), &in, json.MatchCaseInsensitiveNames(true)); err != nil {
		t.Fatal(err)
	}
	if len(in.Tasks) != 1 || in.Tasks[0].ID != "t1" || string(in.Tasks[0].Effort) != "null" {
		t.Fatalf("case-insensitive fields/raw null did not decode: %+v", in)
	}
	if got := effortLevel(in.Tasks[0].Effort); got != "" {
		t.Errorf("null effort = %q, want no effort label", got)
	}
}

func TestStatusColor(t *testing.T) {
	t.Parallel()

	if statusColor("unheard-of") != "" {
		t.Error("unknown status should not be colored")
	}
	if statusColor("running") == "" {
		t.Error("running should be colored")
	}
}
