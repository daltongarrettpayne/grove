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
	for _, r := range rows {
		fmt.Println(r)
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
