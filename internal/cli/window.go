package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/picker"
	"github.com/daltongarrettpayne/grove/internal/scanner"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Manage grove windows",
	Long: `Commands for listing and navigating the windows in a grove context session.

Windows in grove follow the display grammar: "home" (lane 0, always first), then
one row per code lane in the format "<repo>  ·  <branch>", with repo names
left-padded so the separator columns align.`,
}

var windowListCmd = &cobra.Command{
	Use:   "list [<context>]",
	Short: "List windows for a context (defaults to current tmux session)",
	Long: `Print the ordered list of windows for a context, one per line.

Preconditions:
  - <context> must exist in $GROVE_HOME_ROOT/01-Projects/ or 02-Areas/.
  - If no <context> is given, must be run inside a tmux session (TMUX must be set).

Output format:
  - "home" is always first.
  - Code lanes follow as "<repo>  ·  <branch>" with aligned separators.
  - When listing the current session, the active window is prefixed with "* ";
    all others are prefixed with "  ".

When <context> is given explicitly, no "current window" marking is applied
unless the named context matches the session you are currently attached to.`,
	Example: `  # List windows for the current session (must be inside tmux):
  grove window list

  # List windows for a named context (works outside tmux):
  grove window list kalashnikov

  # Pipe to fzf manually:
  grove window list | fzf`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return runWindowList(args[0])
		}
		// No arg: derive context from the current tmux session.
		if os.Getenv("TMUX") == "" {
			return fmt.Errorf("no context specified and not inside a tmux session")
		}
		name, err := tmux.CurrentSessionName()
		if err != nil {
			return fmt.Errorf("getting current session name: %w", err)
		}
		return runWindowList(name)
	},
}

var windowPickCmd = &cobra.Command{
	Use:   "pick",
	Short: "Open the fzf window picker for the current session",
	Long: `Open the interactive picker over all windows in the current grove session,
then switch to the selected window.

Preconditions:
  - Must be run inside a tmux session (TMUX must be set).
  - The picker binary (default: fzf) must be on PATH. Override with GROVE_PICKER.
  - The current session name must match a context in the vault tiers.

What it does:
  1. Derives the context name from the current tmux session.
  2. Builds the ordered window list: "home" first, then code lanes.
  3. Passes the list to the picker binary via stdin.
  4. On selection, switches to the chosen tmux window by name.
  5. On cancel (Esc / Ctrl-C), exits silently with code 0.

This command is designed to be bound to a tmux key in your tmux.conf:
  bind-key w run-shell "grove window pick"`,
	Example: `  # Open the picker (inside tmux):
  grove window pick

  # Use a different picker:
  GROVE_PICKER=sk grove window pick

  # Typical tmux.conf binding:
  bind-key w run-shell "grove window pick"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWindowPicker()
	},
}

func init() {
	windowCmd.AddCommand(windowListCmd)
	windowCmd.AddCommand(windowPickCmd)
	rootCmd.AddCommand(windowCmd)
}

// buildWindowRows returns the ordered picker rows for a context: "home" first,
// then one row per code lane. Also returns the underlying lanes for callers
// that need them (e.g. runWindowPicker).
func buildWindowRows(ctxName string) (rows []string, lanes []model.Lane, err error) {
	tiers := []string{"01-Projects", "02-Areas"}
	found := false
	for _, tier := range tiers {
		tierDir := filepath.Join(cfg.HomeRoot, tier)
		entries, readErr := os.ReadDir(tierDir)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			return nil, nil, fmt.Errorf("reading vault tier %s: %w", tierDir, readErr)
		}
		for _, e := range entries {
			if e.IsDir() && e.Name() == ctxName {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("context %q not found in vault", ctxName)
	}

	codeContainer := filepath.Join(cfg.CodeRoot, ctxName)
	if _, statErr := os.Stat(codeContainer); statErr == nil {
		scanned, scanErr := scanner.ScanSourceSet(model.SourceSet{codeContainer})
		if scanErr != nil {
			return nil, nil, fmt.Errorf("scanning %s: %w", codeContainer, scanErr)
		}
		lanes = scanned
	}

	maxRepoLen := 0
	for _, l := range lanes {
		if len(l.Repo) > maxRepoLen {
			maxRepoLen = len(l.Repo)
		}
	}

	rows = make([]string, 0, 1+len(lanes))
	rows = append(rows, "home")
	for _, l := range lanes {
		rows = append(rows, l.DisplayRow(maxRepoLen))
	}
	return rows, lanes, nil
}

func runWindowList(ctxName string) error {
	rows, _, err := buildWindowRows(ctxName)
	if err != nil {
		return err
	}

	// Mark the current window only when listing the current tmux session.
	var curRepo, curBranch string
	var curIsHome bool
	marking := false
	if os.Getenv("TMUX") != "" {
		if sessionName, sErr := tmux.CurrentSessionName(); sErr == nil && sessionName == ctxName {
			if windowName, wErr := tmux.CurrentWindowName(); wErr == nil {
				curRepo, curBranch, curIsHome = model.ParseWindowName(windowName)
				marking = true
			}
		}
	}

	for _, r := range rows {
		if !marking {
			fmt.Println(r)
			continue
		}
		rRepo, rBranch, rIsHome := model.ParseWindowName(r)
		if rIsHome == curIsHome && rRepo == curRepo && rBranch == curBranch {
			fmt.Println("* " + r)
		} else {
			fmt.Println("  " + r)
		}
	}
	return nil
}

func runWindowPicker() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("grove window pick must be run inside a tmux session")
	}

	sessionName, err := tmux.CurrentSessionName()
	if err != nil {
		return fmt.Errorf("getting current session name: %w", err)
	}

	rows, _, err := buildWindowRows(sessionName)
	if err != nil {
		return err
	}

	chosen, err := picker.NewFzf(cfg.Picker).Select(rows)
	if err != nil {
		if errors.Is(err, picker.ErrCancelled) {
			return nil
		}
		return fmt.Errorf("picker: %w", err)
	}

	if err := tmux.SelectWindow(sessionName, chosen); err != nil {
		return fmt.Errorf("selecting window: %w", err)
	}
	return nil
}
