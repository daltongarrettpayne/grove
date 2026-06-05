// Package config handles loading grove's runtime configuration.
// Priority (high → low): CLI flags > environment variables > defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all runtime options for grove. Callers read this struct;
// they never write env vars or flags back into it after Load returns.
type Config struct {
	CodeRoot   string // root directory scanned for code repos, e.g. ~/code
	HomeRoot   string // root of the knowledge/vault tree, e.g. ~/life-vault
	TmuxSocket string // path to a private tmux socket; empty = use the default tmux socket
	Picker     string // picker binary name, default "fzf"
	LogLevel   string // "debug" | "info" | "warn" | "error"
}

// Load builds a Config from environment variables and hard-coded defaults.
// CLI flags are applied by the cobra PersistentPreRunE after this returns.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}

	cfg := &Config{
		CodeRoot: filepath.Join(home, "code"),
		HomeRoot: home,
		Picker:   "fzf",
		LogLevel: "info",
	}

	applyEnv(cfg)
	return cfg, nil
}

// applyEnv overrides cfg fields with any GROVE_* env vars that are set.
func applyEnv(cfg *Config) {
	if v := os.Getenv("GROVE_CODE_ROOT"); v != "" {
		cfg.CodeRoot = v
	}
	if v := os.Getenv("GROVE_HOME_ROOT"); v != "" {
		cfg.HomeRoot = v
	}
	if v := os.Getenv("GROVE_TMUX_SOCKET"); v != "" {
		cfg.TmuxSocket = v
	}
	if v := os.Getenv("GROVE_PICKER"); v != "" {
		cfg.Picker = v
	}
	if v := os.Getenv("GROVE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
}
