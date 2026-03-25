package claudehooks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	baseConfigName  = "permission-gate.json"
	localConfigName = "permission-gate.local.json"
)

type Config struct {
	Provider     ProviderConfig `json:"provider"`
	TrustedPaths []string       `json:"trusted_paths"`
	PreToolDeny  []PreToolRule  `json:"pre_tool_deny"`
	Allow        []string       `json:"allow"`
	SoftDeny     []string       `json:"soft_deny"`
	Environment  []string       `json:"environment"`
}

type ProviderConfig struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	TimeoutMS int    `json:"timeout_ms"`
}

type PreToolRule struct {
	Matcher           string `json:"matcher"`
	Pattern           string `json:"pattern"`
	Reason            string `json:"reason"`
	AdditionalContext string `json:"additional_context"`
}

func DefaultConfig() Config {
	return Config{
		Provider: ProviderConfig{
			Name:      "anthropic",
			Model:     "claude-opus-4-6",
			TimeoutMS: 4000,
		},
		TrustedPaths: []string{"~/.claude", "~/.codex"},
	}
}

func LoadConfig() (Config, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, err
	}

	basePath := filepath.Join(home, ".claude", baseConfigName)
	localPath := filepath.Join(home, ".claude", localConfigName)

	if err := mergeConfigFile(basePath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	if err := mergeConfigFile(localPath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}

	if cfg.Provider.TimeoutMS <= 0 {
		cfg.Provider.TimeoutMS = 4000
	}

	return cfg, nil
}

func mergeConfigFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var override Config
	if err := json.Unmarshal(data, &override); err != nil {
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

	cfg.TrustedPaths = append(cfg.TrustedPaths, override.TrustedPaths...)
	cfg.PreToolDeny = append(cfg.PreToolDeny, override.PreToolDeny...)
	cfg.Allow = append(cfg.Allow, override.Allow...)
	cfg.SoftDeny = append(cfg.SoftDeny, override.SoftDeny...)
	cfg.Environment = append(cfg.Environment, override.Environment...)

	return nil
}

func ExpandPath(path string, cwd string) string {
	if path == "" {
		return ""
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if cwd == "" {
		abs, err := filepath.Abs(path)
		if err == nil {
			return filepath.Clean(abs)
		}
		return filepath.Clean(path)
	}

	return filepath.Clean(filepath.Join(cwd, path))
}
