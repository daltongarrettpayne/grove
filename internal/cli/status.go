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
