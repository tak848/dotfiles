package claudehooks

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	jsonnet "github.com/google/go-jsonnet"
)

const (
	baseConfigName  = "permission-gate.jsonnet"
	localConfigName = "permission-gate.local.jsonnet"
)

type Config struct {
	Provider    ProviderConfig `json:"provider"`
	PreToolDeny []PreToolRule  `json:"pre_tool_deny"`
	Allow       []string       `json:"allow"`
	SoftDeny    []string       `json:"soft_deny"`
	Environment []string       `json:"environment"`
}

type ProviderConfig struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	TimeoutMS int    `json:"timeout_ms"`
}

type PreToolRule struct {
	Matcher       string `json:"matcher"`
	Pattern       string `json:"pattern"`
	Reason        string `json:"reason"`
	SystemMessage string `json:"system_message"`
}

func DefaultConfig() Config {
	return Config{
		Provider: ProviderConfig{
			Name:      "anthropic",
			Model:     string(anthropic.ModelClaudeHaiku4_5),
			TimeoutMS: 4000,
		},
	}
}

func LoadConfig(cwd string) (Config, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, err
	}

	basePath := filepath.Join(home, ".claude", baseConfigName)
	if err := mergeConfigFile(basePath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}

	for _, path := range safeProjectLocalConfigPaths(cwd) {
		if err := mergeConfigFile(path, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
			return cfg, err
		}
	}

	if cfg.Provider.TimeoutMS <= 0 {
		cfg.Provider.TimeoutMS = 4000
	}

	return cfg, nil
}

func projectLocalConfigPaths(cwd string) []string {
	var paths []string
	if cwd == "" {
		return paths
	}

	root := cwd
	if repoRoot, err := gitOutput(cwd, "rev-parse", "--show-toplevel"); err == nil && repoRoot != "" {
		root = repoRoot
	}

	paths = append(paths,
		filepath.Join(root, localConfigName),
		filepath.Join(root, ".claude", localConfigName),
	)

	return paths
}

func safeProjectLocalConfigPaths(cwd string) []string {
	root := cwd
	if repoRoot, err := gitOutput(cwd, "rev-parse", "--show-toplevel"); err == nil && repoRoot != "" {
		root = repoRoot
	}

	var safe []string
	for _, path := range projectLocalConfigPaths(cwd) {
		tracked, err := isTrackedProjectFile(root, path)
		if err != nil || tracked {
			continue
		}
		safe = append(safe, path)
	}
	return safe
}

func isTrackedProjectFile(root string, path string) (bool, error) {
	if root == "" {
		return false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, nil
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}

	cmd := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", rel)
	if err := cmd.Run(); err == nil {
		return true, nil
	}
	return false, nil
}

func mergeConfigFile(path string, cfg *Config) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}

	vm := jsonnet.MakeVM()
	data, err := vm.EvaluateFile(path)
	if err != nil {
		return err
	}

	var override Config
	if err := json.Unmarshal([]byte(data), &override); err != nil {
		return err
	}

	if override.Provider.Name != "" {
		cfg.Provider.Name = override.Provider.Name
	}
	if override.Provider.Model != "" {
		cfg.Provider.Model = override.Provider.Model
	}
	if override.Provider.TimeoutMS > 0 {
		cfg.Provider.TimeoutMS = override.Provider.TimeoutMS
	}

	cfg.PreToolDeny = append(cfg.PreToolDeny, override.PreToolDeny...)
	cfg.Allow = append(cfg.Allow, override.Allow...)
	cfg.SoftDeny = append(cfg.SoftDeny, override.SoftDeny...)
	cfg.Environment = append(cfg.Environment, override.Environment...)

	return nil
}
