package tts

import (
	"testing"
)

func TestHomeDir(t *testing.T) {
	t.Parallel()

	h := homeDir()
	if h == "" {
		t.Error("homeDir() returned empty string")
	}
}
