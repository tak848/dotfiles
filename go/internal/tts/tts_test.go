package tts

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSynthesizeJSON(t *testing.T) {
	text := "<>&" + string([]rune{0x2028, 0x2029})
	// プロセス内の transport だけを差し替え、Google API や音声再生は実行しない。
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tt := range []struct {
		name, response, want string
	}{
		{"audio", `{"audioContent":"bXAz"}`, "mp3"},
		{"case insensitive", `{"AUDIOCONTENT":"bXAz","future":{}}`, "mp3"},
		{"null", `{"audioContent":null}`, ""},
		{"missing", `{}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				for _, escaped := range []string{"\\u003c", "\\u003e", "\\u0026", "\\u2028", "\\u2029"} {
					if !strings.Contains(string(body), escaped) {
						t.Errorf("missing escaping %q in %s", escaped, body)
					}
				}
				var got ttsRequest
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got.Input.Text != text || got.Voice.LanguageCode != "en-US" || got.AudioConfig.SpeakingRate != speed || got.AudioConfig.AudioEncoding != "MP3" {
					t.Errorf("unexpected request: %+v", got)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.response)), Header: make(http.Header)}, nil
			})
			got, err := synthesize("test-key", text, "en-US", "test-voice")
			if err != nil || string(got) != tt.want {
				t.Fatalf("synthesize = %q, %v; want %q", got, err, tt.want)
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
