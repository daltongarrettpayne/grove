package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
}

var windowListCmd = &cobra.Command{
	Use:   "list [<context>]",
	Short: "List windows for a context (defaults to current tmux session)",
	Args:  cobra.MaximumNArgs(1),
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
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWindowPicker()
	},
}

var windowDeleteCmd = &cobra.Command{
	Use:   "delete [<name>]",
	Short: "Kill a window in the current tmux session",
	Args:  cobra.MaximumNArgs(1),
	Long: `Kill a window in the current tmux session.

Preconditions:
  - Must be run inside a tmux session ($TMUX must be set).

What it does:
  - With no argument: kills the current window.
  - With a name argument: kills the named window in the current session.
    The "home" window cannot be deleted.

Exits 0 on success. Exits non-zero if not inside tmux, if the window is
"home", or if tmux fails to kill the window.`,
	Example: `
  # Delete the current window:
  grove window delete

  # Delete a window by name:
  grove window delete "grove  ·  feat/user-auth"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runWindowDeleteCurrent()
		}
		return runWindowDelete(args[0])
	},
}

func init() {
	windowCmd.AddCommand(windowListCmd)
	windowCmd.AddCommand(windowPickCmd)
	windowCmd.AddCommand(windowDeleteCmd)
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

func runWindowDeleteCurrent() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("grove window delete must be run inside a tmux session")
	}
	// kill-window with no -t kills the current window.
	cmd := exec.Command("tmux", "kill-window")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("killing current window: %w", err)
	}
	fmt.Println("deleted current window")
	return nil
}

func runWindowDelete(name string) error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("grove window delete must be run inside a tmux session")
	}
	if name == "home" {
		return fmt.Errorf("cannot delete the home window")
	}
	sessionName, err := tmux.CurrentSessionName()
	if err != nil {
		return fmt.Errorf("getting current session name: %w", err)
	}
	if err := tmux.KillWindow(sessionName, name); err != nil {
		return fmt.Errorf("killing window %q: %w", name, err)
	}
	fmt.Printf("deleted window %s\n", name)
	return nil
}
