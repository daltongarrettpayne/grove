package config_test

import (
	"strings"
	"testing"

	"github.com/daltongarrettpayne/grove/internal/config"
)

func TestLoad_defaults(t *testing.T) {
	// Clear any GROVE_* env vars that might be set in the environment so the
	// test observes pure defaults regardless of the caller's shell state.
	t.Setenv("GROVE_CODE_ROOT", "")
	t.Setenv("GROVE_HOME_ROOT", "")
	t.Setenv("GROVE_TMUX_SOCKET", "")
	t.Setenv("GROVE_PICKER", "")
	t.Setenv("GROVE_LOG_LEVEL", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !strings.HasSuffix(cfg.CodeRoot, "/code") {
		t.Errorf("CodeRoot = %q, want suffix \"/code\"", cfg.CodeRoot)
	}
	if cfg.HomeRoot == "" {
		t.Errorf("HomeRoot is empty, want non-empty")
	}
	if cfg.Picker != "fzf" {
		t.Errorf("Picker = %q, want %q", cfg.Picker, "fzf")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("GROVE_CODE_ROOT", "/custom/code")
	t.Setenv("GROVE_HOME_ROOT", "/custom/home")
	t.Setenv("GROVE_PICKER", "sk")
	t.Setenv("GROVE_LOG_LEVEL", "debug")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.CodeRoot != "/custom/code" {
		t.Errorf("CodeRoot = %q, want %q", cfg.CodeRoot, "/custom/code")
	}
	if cfg.HomeRoot != "/custom/home" {
		t.Errorf("HomeRoot = %q, want %q", cfg.HomeRoot, "/custom/home")
	}
	if cfg.Picker != "sk" {
		t.Errorf("Picker = %q, want %q", cfg.Picker, "sk")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}
