package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

// statusBarCmd implements `grove status-bar`, a single-call command that
// emits a compact context string for tmux's status-left.
//
// It is designed to be invoked every second by tmux (status-interval 1) and
// must complete in under 100ms.  To achieve this it issues at most two tmux
// display-message calls — one for the session name and one for the window
// name — and does no disk I/O.
//
// Output rules (no trailing newline — tmux appends none of its own):
//   - TMUX not set          → nothing (exit 0)
//   - Home window active    → sessionName
//   - Lane window active    → sessionName + "  ›  " + branch  (if parseable)
//   - Lane window, no parse → sessionName + "  ›  " + windowName verbatim
//
// A window is considered "home" when its name equals "home" or equals the
// session name (the latter handles the case where the user has renamed the
// home window to match the session).
var statusBarCmd = &cobra.Command{
	Use:   "status-bar",
	Short: "Print a compact status line for tmux status-left",
	Long: `Print a single compact line for tmux's status-left setting.

Designed to run on every status-interval tick (typically 1 second).
Issues at most two tmux display-message calls and performs no disk I/O,
keeping runtime well under 100ms.

Output format:
  Outside tmux           → (nothing)
  Home window            → sessionName
  Lane window with branch → sessionName  ›  branch
  Other lane window      → sessionName  ›  windowName

Usage in tmux.conf:
  set -g status-left '#($HOME/.local/bin/grove status-bar)'
  set -g status-left-length 60`,
	// PersistentPreRunE in root.go loads config and sets up logging — both are
	// unnecessary overhead for this hot path.  Override with a no-op so we
	// skip config loading entirely.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatusBar()
	},
}

func init() {
	rootCmd.AddCommand(statusBarCmd)
}

func runStatusBar() error {
	// Exit silently when not running inside a tmux session.
	if os.Getenv("TMUX") == "" {
		return nil
	}

	session, err := tmux.CurrentSessionName()
	if err != nil {
		// Suppress errors — a broken status bar is worse than a silent one.
		return nil
	}

	window, err := tmux.CurrentWindowName()
	if err != nil {
		// Fall back to session name only.
		fmt.Print(session)
		return nil
	}

	// A window is "home" if its name is the literal string "home" or if it
	// matches the session name (grove names the home window after the session
	// in some code paths).
	_, _, isHome := model.ParseWindowName(window)
	if isHome || window == session {
		fmt.Print(session)
		return nil
	}

	// For lane windows, prefer the branch name from the parsed window name.
	// ParseWindowName returns a non-empty branch only when the window name
	// follows the grove "repo  ·  branch" convention.
	_, branch, _ := model.ParseWindowName(window)
	if branch != "" {
		fmt.Print(session + "  ›  " + branch)
		return nil
	}

	// Window name doesn't follow grove's convention — use it verbatim.
	fmt.Print(session + "  ›  " + window)
	return nil
}
