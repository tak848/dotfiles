package claudehooks

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	ToolInput             map[string]any    `json:"tool_input"`
	PermissionSuggestions []json.RawMessage `json:"permission_suggestions"`
}

type PreToolDecision struct {
	Reason            string
	AdditionalContext string
}

func EvaluatePreTool(cfg Config, input HookInput) *PreToolDecision {
	if decision := matchPreToolRules(cfg.PreToolDeny, input); decision != nil {
		return decision
	}

	candidates := CandidatePaths(input)
	if len(candidates) == 0 {
		return nil
	}

	roots := TrustedRoots(cfg, input.Cwd)
	for _, candidate := range candidates {
		if !isTrusted(candidate, roots, input.Cwd) {
			return &PreToolDecision{
				Reason:            "trusted path 外へのアクセスは拒否",
				AdditionalContext: "現在の作業ディレクトリと信頼済みローカル設定ディレクトリの外にある path へアクセスしようとしました。必要なら対象 path を明示してユーザー確認を取ってください。",
			}
		}
	}

	return nil
}

func TrustedRoots(cfg Config, cwd string) []string {
	var roots []string
	if cwd != "" {
		roots = append(roots, canonicalPath(cwd, cwd))
		if repoRoot, err := gitOutput(cwd, "rev-parse", "--show-toplevel"); err == nil && repoRoot != "" {
			roots = append(roots, canonicalPath(repoRoot, cwd))
		}
	}

	for _, configured := range cfg.TrustedPaths {
		roots = append(roots, canonicalPath(configured, cwd))
	}

	return uniqueNonEmpty(roots)
}

func CandidatePaths(input HookInput) []string {
	switch input.ToolName {
	case "Read", "Write", "Edit", "MultiEdit":
		return stringsToPaths(input.Cwd, stringValue(input.ToolInput, "file_path"))
	case "Glob", "Grep":
		return stringsToPaths(input.Cwd, stringValue(input.ToolInput, "path"))
	case "Bash":
		return extractBashPaths(input.Cwd, stringValue(input.ToolInput, "command"))
	default:
		return nil
	}
}

func matchPreToolRules(rules []PreToolRule, input HookInput) *PreToolDecision {
	command := stringValue(input.ToolInput, "command")
	for _, rule := range rules {
		if rule.Matcher != "" && rule.Matcher != "*" {
			matched, err := regexp.MatchString(rule.Matcher, input.ToolName)
			if err != nil || !matched {
				continue
			}
		}
		if command == "" || rule.Pattern == "" {
			continue
		}
		matched, err := regexp.MatchString(rule.Pattern, command)
		if err != nil || !matched {
			continue
		}
		return &PreToolDecision{
			Reason:            rule.Reason,
			AdditionalContext: rule.AdditionalContext,
		}
	}
	return nil
}

func isTrusted(path string, roots []string, cwd string) bool {
	resolved := canonicalPath(path, cwd)
	for _, root := range roots {
		if root == "" {
			continue
		}
		if resolved == root || strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func canonicalPath(path string, cwd string) string {
	expanded := ExpandPath(path, cwd)
	if expanded == "" {
		return ""
	}

	if resolved, err := filepath.EvalSymlinks(expanded); err == nil {
		return filepath.Clean(resolved)
	}

	current := expanded
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(expanded)
		}
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			rel, relErr := filepath.Rel(parent, expanded)
			if relErr != nil {
				return filepath.Clean(expanded)
			}
			return filepath.Clean(filepath.Join(resolved, rel))
		}
		current = parent
	}
}

func extractBashPaths(cwd string, command string) []string {
	tokens := shellSplit(command)
	if len(tokens) == 0 {
		return nil
	}

	var candidates []string
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "" || strings.HasPrefix(token, "-") {
			continue
		}

		if token == "git" && i+2 < len(tokens) && tokens[i+1] == "-C" {
			candidates = append(candidates, tokens[i+2])
			i += 2
			continue
		}

		if token == "cd" && i+1 < len(tokens) {
			candidates = append(candidates, tokens[i+1])
			i++
			continue
		}

		if looksLikePathToken(token) {
			candidates = append(candidates, token)
		}
	}

	return stringsToPaths(cwd, candidates...)
}

func looksLikePathToken(token string) bool {
	if token == "." || token == ".." {
		return true
	}
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "./") || strings.HasPrefix(token, "../") || strings.HasPrefix(token, "~/") {
		return true
	}
	return false
}

func stringsToPaths(cwd string, values ...string) []string {
	var paths []string
	for _, value := range values {
		if value == "" {
			continue
		}
		paths = append(paths, canonicalPath(value, cwd))
	}
	return uniqueNonEmpty(paths)
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

func stringValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	value, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := value.(string)
	return s
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
