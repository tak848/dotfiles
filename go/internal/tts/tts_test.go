package tts

import (
	"testing"
)

func TestIsJapanese(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  bool
	}{
		"ascii":    {input: "hello world", want: false},
		"hiragana": {input: "こんにちは", want: true},
		"katakana": {input: "カタカナ", want: true},
		"kanji":    {input: "漢字", want: true},
		"mixed":    {input: "Hello こんにちは", want: true},
		"empty":    {input: "", want: false},
		"emoji":    {input: "🎉", want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := IsJapanese(tt.input); got != tt.want {
				t.Errorf("IsJapanese(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHomeDir(t *testing.T) {
	t.Parallel()

	h := homeDir()
	if h == "" {
		t.Error("homeDir() returned empty string")
	}
}
