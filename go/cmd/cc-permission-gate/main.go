package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tak848/dotfiles/go/internal/claudehooks"
)

func main() {
	logger, cleanup := initLogger()
	defer cleanup()
	slog.SetDefault(logger)

	var input claudehooks.HookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		logger.Error("failed to decode stdin", "error", err)
		return
	}

	logger.Info("hook invoked",
		"tool", input.ToolName,
		"permission_mode", input.PermissionMode,
	)

	cfg, err := claudehooks.LoadConfig(input.Cwd)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return
	}

	start := time.Now()
	decision, ok, err := claudehooks.DecidePermission(context.Background(), cfg, input)
	elapsed := time.Since(start)

	if err != nil {
		logger.Error("DecidePermission failed",
			"error", err,
			"tool", input.ToolName,
			"elapsed_ms", elapsed.Milliseconds(),
		)
		return
	}
	if !ok {
		logger.Info("DecidePermission: no decision (fallthrough)",
			"tool", input.ToolName,
			"elapsed_ms", elapsed.Milliseconds(),
		)
		return
	}

	logger.Info("DecidePermission: decision made",
		"behavior", decision.Behavior,
		"message", decision.Message,
		"tool", input.ToolName,
		"elapsed_ms", elapsed.Milliseconds(),
	)

	output := permissionRequestResponse{
		HookSpecificOutput: permissionRequestOutput{
			HookEventName: "PermissionRequest",
			Decision: permissionDecisionOutput{
				Behavior: decision.Behavior,
				Message:  decision.Message,
			},
		},
	}

	_ = json.NewEncoder(os.Stdout).Encode(output)
}

const maxLogSize = 5 * 1024 * 1024 // 5 MB

func initLogger() (*slog.Logger, func()) {
	home, err := os.UserHomeDir()
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() {}
	}

	logDir := filepath.Join(home, ".claude", "logs")
	_ = os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "permission-gate.log")
	rotateLog(logPath)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() {}
	}

	w := &atomicWriter{f: f}
	return slog.New(slog.NewTextHandler(w, nil)), func() { f.Close() }
}

type atomicWriter struct {
	f  *os.File
	mu sync.Mutex
}

func (w *atomicWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var buf bytes.Buffer
	buf.Write(p)
	return w.f.Write(buf.Bytes())
}

func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxLogSize {
		return
	}
	prev := path + ".1"
	_ = os.Remove(prev)
	_ = os.Rename(path, prev)
}

type permissionRequestResponse struct {
	HookSpecificOutput permissionRequestOutput `json:"hookSpecificOutput"`
}

type permissionRequestOutput struct {
	HookEventName string                   `json:"hookEventName"`
	Decision      permissionDecisionOutput `json:"decision"`
}

type permissionDecisionOutput struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}
