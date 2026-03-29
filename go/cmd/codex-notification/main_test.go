package main

import "testing"

func TestNotificationMessageFallback(t *testing.T) {
	t.Parallel()

	got := notificationMessage(Input{})
	want := "Codex の応答が終わりました。"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNotificationMessageUsesFirstLine(t *testing.T) {
	t.Parallel()

	got := notificationMessage(Input{
		Type:                 "agent-turn-complete",
		LastAssistantMessage: "1 行目\n2 行目",
	})
	want := "Codex の応答が終わりました。 1 行目"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSummarizeAssistantMessageTruncates(t *testing.T) {
	t.Parallel()

	got := summarizeAssistantMessage("1234567890", 5)
	want := "1234…"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
