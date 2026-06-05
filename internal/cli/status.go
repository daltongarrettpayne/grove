package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

var statusSegmentCmd = &cobra.Command{
	Use:   "status-segment",
	Short: "Print the tmux status-right segment for the current window",
	Long: `Print a single line of text suitable for embedding in tmux's status-right.

Preconditions:
  - Designed to be called from tmux's status-right via run-shell.
  - If TMUX is not set (called outside tmux), prints an empty line and exits 0.

Output format depends on the current window name:
  "home" window:              "<session> > home"
  code lane window:           "<session> > <repo> . <branch>"
  unrecognised window name:   "<session> > <raw-window-name>"

The separator characters are Unicode: > (U+203A) and . (U+00B7).

This command replaces ad-hoc status scripts. Add it to tmux.conf:
  set -g status-right "#(grove status-segment)"`,
	Example: `  # Typical tmux.conf usage:
  set -g status-right "#(grove status-segment)"

  # Test the output from a terminal inside tmux:
  grove status-segment

  # Test from outside tmux (prints empty line):
  grove status-segment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatusSegment()
	},
}

func init() {
	rootCmd.AddCommand(statusSegmentCmd)
}

func runStatusSegment() error {
	if os.Getenv("TMUX") == "" {
		fmt.Println("")
		return nil
	}

	session, err := tmux.CurrentSessionName()
	if err != nil {
		return fmt.Errorf("getting current session name: %w", err)
	}

	window, err := tmux.CurrentWindowName()
	if err != nil {
		return fmt.Errorf("getting current window name: %w", err)
	}

	repo, branch, isHome := model.ParseWindowName(window)
	switch {
	case isHome:
		fmt.Printf("%s › home\n", session)
	case branch != "":
		fmt.Printf("%s › %s · %s\n", session, repo, branch)
	default:
		// Unrecognised window name (manually created): fall back to raw name.
		fmt.Printf("%s › %s\n", session, window)
	}
	return nil
}
