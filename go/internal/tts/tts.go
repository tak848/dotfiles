package tts

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

const (
	apiURL = "https://texttospeech.googleapis.com/v1/text:synthesize"
	speed  = 1.3
)

// VoicePair holds TTS voice names for Japanese and English.
type VoicePair struct {
	JA string
	EN string
}

// DefaultVoices uses Chirp3-HD voices (notification).
var DefaultVoices = VoicePair{
	JA: "ja-JP-Chirp3-HD-Aoede",
	EN: "en-US-Chirp3-HD-Aoede",
}

// Neural2Voices uses Neural2 voices (stop).
var Neural2Voices = VoicePair{
	JA: "ja-JP-Neural2-B",
	EN: "en-US-Neural2-D",
}

// Speak synthesizes and plays the message via Google Cloud TTS.
// Falls back to the macOS say command if GOOGLE_API_KEY is unset or API fails.
func Speak(message string, voices VoicePair) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		sayFallback(message)
		return
	}

	isASCII := true
	for _, r := range message {
		if r >= 128 {
			isASCII = false
			break
		}
	}

	voice := voices.JA
	if isASCII {
		voice = voices.EN
	}
	langCode := voice[:5]

	cacheDir := filepath.Join(homeDir(), ".cache", "gtts")
	_ = os.MkdirAll(cacheDir, 0o755)

	cacheKey := fmt.Sprintf("%s|%s|%.1f", message, voice, speed)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(cacheKey)))
	cacheFile := filepath.Join(cacheDir, hash+".mp3")

	if _, err := os.Stat(cacheFile); err == nil {
		playAudio(cacheFile)
		return
	}

	audio, err := synthesize(apiKey, message, langCode, voice)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: TTS API failed: %v\n", err)
		sayFallback(message)
		return
	}

	if err := os.WriteFile(cacheFile, audio, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cache write failed: %v\n", err)
	}
	playAudio(cacheFile)
}

// GitContext returns "from repo/branch (worktree)" or "from repo/branch".
func GitContext() string {
	toplevel, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	repoName := filepath.Base(toplevel)

	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}

	gitDir, err := gitOutput("rev-parse", "--git-dir")
	if err != nil {
		return ""
	}

	if strings.Contains(gitDir, ".git/worktrees/") {
		return fmt.Sprintf("from %s/%s (worktree)", repoName, branch)
	}
	return fmt.Sprintf("from %s/%s", repoName, branch)
}

// IsJapanese returns true if the string contains any Japanese characters.
func IsJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

func synthesize(apiKey, text, langCode, voice string) ([]byte, error) {
	payload := map[string]any{
		"input":       map[string]string{"text": text},
		"voice":       map[string]string{"languageCode": langCode, "name": voice},
		"audioConfig": map[string]any{"audioEncoding": "MP3", "speakingRate": speed},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Goog-Api-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(result.AudioContent)
}

// playAudio starts the audio player as a detached background process and returns immediately.
func playAudio(path string) {
	players := [][]string{
		{"mpv", "--no-terminal", "--no-video"},
		{"afplay"},
		{"play", "-q"},
	}
	for _, p := range players {
		if _, err := exec.LookPath(p[0]); err == nil {
			cmd := exec.Command(p[0], append(p[1:], path)...)
			_ = cmd.Start() // non-blocking: subprocess continues after parent exits
			return
		}
	}
}

// sayFallback starts macOS say command as a background process and returns immediately.
func sayFallback(message string) {
	if _, err := exec.LookPath("say"); err != nil {
		return
	}
	cmd := exec.Command("say", message)
	_ = cmd.Start() // non-blocking
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}
