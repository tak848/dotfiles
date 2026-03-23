package main

import (
	"testing"
)

func TestEditToolsSet(t *testing.T) {
	t.Parallel()

	editTools := map[string]struct{}{
		"Write":     {},
		"Edit":      {},
		"MultiEdit": {},
	}

	tests := map[string]struct {
		key  string
		want bool
	}{
		"Write":     {key: "Write", want: true},
		"Edit":      {key: "Edit", want: true},
		"MultiEdit": {key: "MultiEdit", want: true},
		"Read":      {key: "Read", want: false},
		"Bash":      {key: "Bash", want: false},
		"empty":     {key: "", want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, ok := editTools[tt.key]
			if ok != tt.want {
				t.Errorf("editTools[%q] = %v, want %v", tt.key, ok, tt.want)
			}
		})
	}
}
