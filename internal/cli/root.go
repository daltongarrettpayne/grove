// Package cli wires up the cobra command tree.
// root.go defines the root command, global flags, and the PersistentPreRunE
// that loads config and initialises the logger before any subcommand runs.
package cli

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/config"
	grovelog "github.com/daltongarrettpayne/grove/internal/log"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

// cfg is populated by PersistentPreRunE and is readable by all subcommands
// in this package via the shared package-level variable.
var (
	cfg     *config.Config
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "grove",
	Short: "Weave your knowledge tree and code tree into tmux sessions",
	Long: `Grove maps your on-disk structure to tmux sessions deterministically.

A context is a tmux session. Its lanes are windows. Lane 0 is always "home".
Every other lane is a git working tree identified by (repo, branch). The session
is a pure function of disk state — stateless, idempotent, and reconstructible.

Required env vars:
  GROVE_CODE_ROOT   root directory scanned for code repos (default: ~/code)
  GROVE_HOME_ROOT   root of the knowledge/vault tree (default: ~)

Grove scans $GROVE_HOME_ROOT/01-Projects/ and $GROVE_HOME_ROOT/02-Areas/ for
context directories. If a directory named <ctx> exists under $GROVE_CODE_ROOT,
its working trees become lanes in that context's session.`,
	Example: `  # Open a session for the "grove" context:
  grove session open grove

  # List all contexts as JSON:
  grove session list

  # Open the window picker inside a session:
  grove window pick

  # Audit your workspace for convention violations:
  grove doctor`,
	// SilenceUsage prevents cobra from printing usage on every error —
	// usage is only relevant for wrong flags, not runtime errors.
	SilenceUsage: true,
	// SilenceErrors lets main.go control the error message format.
	SilenceErrors: true,
	// PersistentPreRunE runs before every subcommand. It loads config and
	// sets up the global slog logger so all subcommands inherit the same setup.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return err
		}
		// Point every tmux shell-out at the configured socket (if any) so grove
		// addresses the same server whether or not it runs inside tmux.
		tmux.SetSocket(cfg.TmuxSocket)
		level := cfg.LogLevel
		if verbose {
			level = "debug"
		}


		// Open a JSON log file in ~/.local/share/grove/grove.log so every
		// grove invocation is fully captured for debugging.
		var fileOut *os.File
		logDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "grove")
		if mkErr := os.MkdirAll(logDir, 0755); mkErr == nil {
			f, fErr := os.OpenFile(filepath.Join(logDir, "grove.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if fErr == nil {
				fileOut = f
			}
		}
		grovelog.Setup(level, fileOut)
		slog.Debug("grove invoked", "cmd", cmd.Name(), "log_level", level)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
}

// Execute is the single entry point called from cmd/grove/main.go.
func Execute() error {
	return rootCmd.Execute()
}
