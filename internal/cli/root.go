// Package cli wires up the cobra command tree.
// root.go defines the root command, global flags, and the PersistentPreRunE
// that loads config and initialises the logger before any subcommand runs.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/config"
	grovelog "github.com/daltongarrettpayne/grove/internal/log"
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
		level := cfg.LogLevel
		if verbose {
			level = "debug"
		}
		grovelog.Setup(level)
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
