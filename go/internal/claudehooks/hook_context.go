package claudehooks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type HookInput struct {
	SessionID             string            `json:"session_id"`
	TranscriptPath        string            `json:"transcript_path"`
	Cwd                   string            `json:"cwd"`
	PermissionMode        string            `json:"permission_mode"`
	HookEventName         string            `json:"hook_event_name"`
	ToolName              string            `json:"tool_name"`
	ToolInput             HookToolInput     `json:"-"`
	ToolInputRaw          json.RawMessage   `json:"tool_input"`
	PermissionSuggestions []json.RawMessage `json:"permission_suggestions"`
}

type HookToolInput struct {
	Command        string              `json:"command"`
	FilePath       string              `json:"file_path"`
	Path           string              `json:"path"`
	Pattern        string              `json:"pattern"`
	Content        string              `json:"content"`
	ContentUpdates []HookContentUpdate `json:"content_updates"`
}

type HookContentUpdate struct {
	OldString string `json:"old_str"`
	NewString string `json:"new_str"`
}

// RecentTranscript holds recent user messages and tool operations from the session transcript.
type RecentTranscript struct {
	UserMessages    []string `json:"user_messages,omitempty"`
	RecentToolCalls []string `json:"recent_tool_calls,omitempty"`
}

const maxTranscriptLines = 200

// LoadRecentTranscript reads the tail of the transcript JSONL and extracts
// recent user messages and tool call summaries.
func LoadRecentTranscript(path string) RecentTranscript {
	if path == "" {
		return RecentTranscript{}
	}

	f, err := os.Open(path)
	if err != nil {
		return RecentTranscript{}
	}
	defer f.Close()

	// Read all lines, keep last maxTranscriptLines
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer per line
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > maxTranscriptLines {
		lines = lines[len(lines)-maxTranscriptLines:]
	}

	var result RecentTranscript
	for _, line := range lines {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
			ToolName string `json:"tool_name"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		switch {
		case entry.Type == "user" || (entry.Message.Role == "user"):
			if s, ok := entry.Message.Content.(string); ok && s != "" {
				// Keep last 5 user messages
				result.UserMessages = append(result.UserMessages, truncate(s, 200))
				if len(result.UserMessages) > 5 {
					result.UserMessages = result.UserMessages[len(result.UserMessages)-5:]
				}
			}
		case entry.ToolName != "":
			summary := entry.ToolName
			result.RecentToolCalls = append(result.RecentToolCalls, summary)
			if len(result.RecentToolCalls) > 10 {
				result.RecentToolCalls = result.RecentToolCalls[len(result.RecentToolCalls)-10:]
			}
		}
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

type SettingsPermissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

func LoadSettingsPermissions(cwd string) SettingsPermissions {
	home, err := os.UserHomeDir()
	if err != nil {
		return SettingsPermissions{}
	}

	repoRoot := cwd
	if root, err := gitOutput(cwd, "rev-parse", "--show-toplevel"); err == nil && root != "" {
		repoRoot = root
	}

	paths := []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(repoRoot, ".claude", "settings.json"),
		filepath.Join(repoRoot, ".claude", "settings.local.json"),
	}

	var merged SettingsPermissions
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s struct {
			Permissions SettingsPermissions `json:"permissions"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		merged.Allow = append(merged.Allow, s.Permissions.Allow...)
		merged.Deny = append(merged.Deny, s.Permissions.Deny...)
	}
	return merged
}

type PermissionContext struct {
	Cwd                 string   `json:"cwd"`
	RepoRoot            string   `json:"repo_root,omitempty"`
	GitDir              string   `json:"git_dir,omitempty"`
	GitCommonDir        string   `json:"git_common_dir,omitempty"`
	PrimaryCheckoutRoot string   `json:"primary_checkout_root,omitempty"`
	BranchName          string   `json:"branch_name,omitempty"`
	IsWorktree          bool     `json:"is_worktree"`
	ReferencedPaths     []string `json:"referenced_paths,omitempty"`
}

func (h *HookInput) UnmarshalJSON(data []byte) error {
	type alias HookInput
	var raw struct {
		alias
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*h = HookInput(raw.alias)
	h.ToolInputRaw = raw.ToolInput
	if len(raw.ToolInput) > 0 {
		if err := json.Unmarshal(raw.ToolInput, &h.ToolInput); err != nil {
			return err
		}
	}
	return nil
}

func (h HookInput) ToolInputText() string {
	var parts []string
	if h.ToolInput.Command != "" {
		parts = append(parts, h.ToolInput.Command)
	}
	if h.ToolInput.FilePath != "" {
		parts = append(parts, h.ToolInput.FilePath)
	}
	if h.ToolInput.Path != "" {
		parts = append(parts, h.ToolInput.Path)
	}
	if h.ToolInput.Pattern != "" {
		parts = append(parts, h.ToolInput.Pattern)
	}
	if h.ToolInput.Content != "" {
		parts = append(parts, h.ToolInput.Content)
	}
	for _, update := range h.ToolInput.ContentUpdates {
		if update.OldString != "" {
			parts = append(parts, update.OldString)
		}
		if update.NewString != "" {
			parts = append(parts, update.NewString)
		}
	}
	if len(h.ToolInputRaw) > 0 {
		parts = append(parts, string(h.ToolInputRaw))
	}
	return strings.Join(parts, "\n")
}

func BuildPermissionContext(input HookInput) PermissionContext {
	ctx := PermissionContext{
		Cwd:             input.Cwd,
		ReferencedPaths: referencedPaths(input),
	}

	if input.Cwd == "" {
		return ctx
	}

	if repoRoot, err := gitOutput(input.Cwd, "rev-parse", "--show-toplevel"); err == nil {
		ctx.RepoRoot = repoRoot
	}
	if gitDir, err := gitOutput(input.Cwd, "rev-parse", "--git-dir"); err == nil {
		ctx.GitDir = gitDir
		ctx.IsWorktree = strings.Contains(gitDir, ".git/worktrees/")
	}
	if gitCommonDir, err := gitOutput(input.Cwd, "rev-parse", "--git-common-dir"); err == nil {
		ctx.GitCommonDir = gitCommonDir
		if strings.HasSuffix(gitCommonDir, "/.git") || strings.HasSuffix(gitCommonDir, string(filepath.Separator)+".git") {
			ctx.PrimaryCheckoutRoot = filepath.Dir(gitCommonDir)
		}
	}
	if branchName, err := gitOutput(input.Cwd, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		ctx.BranchName = branchName
	}

	return ctx
}

func referencedPaths(input HookInput) []string {
	switch input.ToolName {
	case "Read", "Write", "Edit", "MultiEdit":
		return uniqueNonEmpty(expandPaths(input.Cwd, input.ToolInput.FilePath))
	case "Glob":
		return uniqueNonEmpty(expandPaths(input.Cwd, input.ToolInput.Path, input.ToolInput.Pattern))
	case "Grep":
		return uniqueNonEmpty(expandPaths(input.Cwd, input.ToolInput.Path))
	case "Bash":
		return uniqueNonEmpty(extractBashPaths(input.Cwd, input.ToolInput.Command))
	default:
		return nil
	}
}

func expandPaths(cwd string, values ...string) []string {
	var out []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if after, ok := strings.CutPrefix(value, "~/"); ok {
			if home, err := os.UserHomeDir(); err == nil {
				value = filepath.Join(home, after)
			}
		}
		if filepath.IsAbs(value) {
			out = append(out, filepath.Clean(value))
			continue
		}
		if cwd != "" {
			out = append(out, filepath.Clean(filepath.Join(cwd, value)))
			continue
		}
		out = append(out, filepath.Clean(value))
	}
	return out
}

func extractBashPaths(cwd string, command string) []string {
	tokens := shellSplit(command)
	if len(tokens) == 0 {
		return nil
	}

	var candidates []string
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "" {
			continue
		}

		if token == "git" && i+2 < len(tokens) && tokens[i+1] == "-C" {
			candidates = append(candidates, tokens[i+2])
			i += 2
			continue
		}
		if token == "git" && i+1 < len(tokens) && strings.HasPrefix(tokens[i+1], "-C") && len(tokens[i+1]) > 2 {
			candidates = append(candidates, strings.TrimPrefix(tokens[i+1], "-C"))
			i++
			continue
		}
		if strings.HasPrefix(token, "--") && strings.Contains(token, "=") {
			_, rhs, found := strings.Cut(token, "=")
			if found && looksLikePathToken(rhs) {
				candidates = append(candidates, rhs)
				continue
			}
		}
		if looksLikePathToken(token) {
			candidates = append(candidates, token)
		}
	}

	return expandPaths(cwd, candidates...)
}

func looksLikePathToken(token string) bool {
	if token == "." || token == ".." {
		return true
	}
	return strings.HasPrefix(token, "/") ||
		strings.HasPrefix(token, "./") ||
		strings.HasPrefix(token, "../") ||
		strings.HasPrefix(token, "~/")
}

func uniqueNonEmpty(values []string) []string {
	var out []string
	for _, value := range values {
		if value == "" || slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func shellSplit(input string) []string {
	var fields []string
	var current bytes.Buffer
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		fields = append(fields, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return fields
}

func gitOutput(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
