package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/scanner"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage grove sessions",
	Long: `Commands for building, listing, and removing grove context sessions.

A grove session is a tmux session derived from a context directory in your vault
tree. Every session has a pinned "home" window (lane 0) plus one window per git
working tree found in the matching code directory.`,
}

var sessionOpenCmd = &cobra.Command{
	Use:   "open <context>",
	Short: "Open or attach to a context session (idempotent)",
	Long: `Build a tmux session for <context> and attach to it, or attach if it already exists.

Preconditions:
  - GROVE_HOME_ROOT must be set (or default ~/  must exist).
  - <context> must be a directory under $GROVE_HOME_ROOT/01-Projects/ or
    $GROVE_HOME_ROOT/02-Areas/.
  - tmux must be installed and reachable on PATH.

What it does:
  1. Locates <context> in the vault tiers (01-Projects, then 02-Areas).
  2. If the session already exists, attaches immediately (idempotent).
  3. Otherwise: creates a session rooted at <context>'s vault directory, names
     window 0 "home", scans $GROVE_CODE_ROOT/<context> for git working trees,
     and creates one window per working tree using the "(repo) · (branch)" name
     format. Focus returns to "home" before attach.
  4. Attaches to the session (or switches if already inside tmux).

Exit behaviour: exits 0 on successful attach; non-zero if the context is not
found, the home directory is missing, or tmux fails.`,
	Example: `  # Build and attach to the "kalashnikov" session:
  grove session open kalashnikov

  # Idempotent — safe to run again if the session already exists:
  grove session open kalashnikov

  # Use a custom vault root for a one-off:
  GROVE_HOME_ROOT=~/work-vault grove session open my-project`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionOpen(args[0])
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available contexts as JSON",
	Long: `Scan the vault tiers and emit all available contexts as a JSON array.

Preconditions:
  - GROVE_HOME_ROOT must contain 01-Projects/ and/or 02-Areas/ subdirectories.

Output: a JSON array where each element has:
  - "name":       the context directory name
  - "home_dir":   absolute path to the vault directory
  - "lanes":      array of lane objects (kind, repo, branch, dir)

Contexts from both 01-Projects/ and 02-Areas/ are included. Code lanes are
populated only when a matching directory exists under GROVE_CODE_ROOT.

The output is designed to be consumed by scripts, fzf, or other tooling.`,
	Example: `  # List all contexts:
  grove session list

  # Pretty-print with jq:
  grove session list | jq '.[].name'

  # Find contexts that have code lanes:
  grove session list | jq '[.[] | select(.lanes | length > 0)]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionList()
	},
}

func init() {
	sessionCmd.AddCommand(sessionOpenCmd)
	sessionCmd.AddCommand(sessionListCmd)
	rootCmd.AddCommand(sessionCmd)
}

func runSessionOpen(name string) error {
	// Step 1: Find the context by scanning vault tiers.
	tiers := []string{"01-Projects", "02-Areas"}
	var found *model.Context
	for _, tier := range tiers {
		tierDir := filepath.Join(cfg.HomeRoot, tier)
		entries, err := os.ReadDir(tierDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("reading vault tier %s: %w", tierDir, err)
		}
		for _, e := range entries {
			if !e.IsDir() || isHiddenName(e.Name()) {
				continue
			}
			if e.Name() == name {
				homeDir := filepath.Join(tierDir, name)
				codeContainer := filepath.Join(cfg.CodeRoot, name)
				found = &model.Context{
					Name:      name,
					HomeDir:   homeDir,
					SourceSet: model.SourceSet{codeContainer},
				}
				break
			}
		}
		if found != nil {
			break
		}
	}
	if found == nil {
		return fmt.Errorf("context %q not found", name)
	}

	// Step 2: Check if the session already exists; skip build if so.
	exists, err := tmux.HasSession(name)
	if err != nil {
		return fmt.Errorf("checking session %q: %w", name, err)
	}

	if !exists {
		// Step 3: Validate that the home directory exists on disk.
		if _, statErr := os.Stat(found.HomeDir); statErr != nil {
			return fmt.Errorf("home dir %s does not exist: %w", found.HomeDir, statErr)
		}

		// Step 4a: Create the session rooted at the home directory.
		slog.Info("creating session", "name", name, "home", found.HomeDir)
		if err := tmux.NewSession(name, found.HomeDir); err != nil {
			return fmt.Errorf("creating session %q: %w", name, err)
		}

		// Step 4b: Rename window 0 to "home".
		if err := tmux.RenameWindow(name, "0", "home"); err != nil {
			return fmt.Errorf("renaming home window in %q: %w", name, err)
		}

		// Step 4c: Scan the code container if it exists.
		var lanes []model.Lane
		codeContainer := filepath.Join(cfg.CodeRoot, name)
		if _, statErr := os.Stat(codeContainer); statErr == nil {
			slog.Debug("scanning source set", "container", codeContainer)
			scanned, scanErr := scanner.ScanSourceSet(model.SourceSet{codeContainer})
			if scanErr != nil {
				return fmt.Errorf("scanning %s: %w", codeContainer, scanErr)
			}
			lanes = scanned
		}

		// Step 4d: Create a window per lane using DisplayRow for the name.
		maxRepoLen := 0
		for _, l := range lanes {
			if len(l.Repo) > maxRepoLen {
				maxRepoLen = len(l.Repo)
			}
		}
		for _, lane := range lanes {
			windowName := lane.DisplayRow(maxRepoLen)
			slog.Debug("creating window", "session", name, "window", windowName, "dir", lane.Dir)
			if err := tmux.NewWindow(name, windowName, lane.Dir); err != nil {
				return fmt.Errorf("creating window %q in session %q: %w", windowName, name, err)
			}
		}

		// new-window focuses each window as it is created, so after the loop the
		// active window is the last code lane. Return focus to "home" (window 0)
		// before attaching so the user always lands there.
		if err := tmux.SelectWindow(name, "home"); err != nil {
			return fmt.Errorf("selecting home window in %q: %w", name, err)
		}
	} else {
		slog.Info("session already exists, attaching", "name", name)
	}

	// Step 5: Attach or switch to the session.
	return tmux.AttachOrSwitch(name)
}

func runSessionList() error {
	// Vault tiers whose subdirectories are workspace contexts, per the design doc.
	tiers := []string{"01-Projects", "02-Areas"}

	var contexts []model.Context
	for _, tier := range tiers {
		tierDir := filepath.Join(cfg.HomeRoot, tier)
		entries, err := os.ReadDir(tierDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("reading vault tier %s: %w", tierDir, err)
		}
		for _, e := range entries {
			if !e.IsDir() || isHiddenName(e.Name()) {
				continue
			}
			name := e.Name()
			homeDir := filepath.Join(tierDir, name)

			// lanes is initialised as empty (not nil) so JSON encodes [] not null.
			lanes := make([]model.Lane, 0)
			codeContainer := filepath.Join(cfg.CodeRoot, name)
			if _, statErr := os.Stat(codeContainer); statErr == nil {
				scanned, scanErr := scanner.ScanSourceSet(model.SourceSet{codeContainer})
				if scanErr != nil {
					return fmt.Errorf("scanning %s: %w", codeContainer, scanErr)
				}
				lanes = scanned
			}

			contexts = append(contexts, model.Context{
				Name:      name,
				HomeDir:   homeDir,
				SourceSet: model.SourceSet{codeContainer},
				Lanes:     lanes,
			})
		}
	}

	out, err := json.MarshalIndent(contexts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling contexts: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// isHiddenName reports whether a directory name starts with a dot.
func isHiddenName(name string) bool {
	return len(name) > 0 && name[0] == '.'
}
