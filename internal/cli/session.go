package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/picker"
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

var sessionListJSON bool

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all contexts in the vault",
	Long: `Scan the vault tiers and print all available contexts.

Preconditions:
  - GROVE_HOME_ROOT must contain 01-Projects/ and/or 02-Areas/ subdirectories.

Human output (default): an aligned table with columns:
  name     — context directory name
  tier     — "project" (01-Projects) or "area" (02-Areas)
  lanes    — number of git working trees in the code directory
  session  — whether a live tmux session exists for this context

JSON output (--json): a JSON array where each element has:
  name, tier, lane_count, session, home_dir, lanes[]

Contexts from both 01-Projects/ and 02-Areas/ are included.`,
	Example: `  # Human-readable table:
  grove session list

  # Machine-readable JSON:
  grove session list --json

  # Find live sessions with jq:
  grove session list --json | jq '[.[] | select(.session == true)]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionList(sessionListJSON)
	},
}

var sessionDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Kill a running tmux session by name",
	Args:  cobra.ExactArgs(1),
	Long: `Kill a running tmux session by name.

Preconditions:
  - The named session must currently exist in tmux.

What it does:
  1. Verifies the session exists (fails loudly if not).
  2. Runs tmux kill-session to terminate it.
  3. Prints confirmation.

Exits 0 on success. Exits non-zero if the session is not found or kill fails.`,
	Example: `
  # Delete a session named "grove":
  grove session delete grove

  # Delete the session for a project context:
  grove session delete Kalashnikov.AI`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionDelete(args[0])
	},
}

var sessionPickCmd = &cobra.Command{
	Use:   "pick",
	Short: "Open the fzf session picker and switch to the selected context",
	Long: `Open an interactive picker over all vault contexts, then open or switch to
the chosen grove session.

Preconditions:
  - The picker binary (default: fzf) must be on PATH. Override with GROVE_PICKER.
  - GROVE_HOME_ROOT must contain 01-Projects/ and/or 02-Areas/ subdirectories.

When invoked inside tmux (and not already inside a popup), grove re-invokes
itself inside a tmux display-popup so the picker floats over the current window.

What it does:
  1. Scans 01-Projects/ and 02-Areas/ under GROVE_HOME_ROOT for context directories.
  2. Builds a row per context: name, tier, lane count, live-session indicator.
  3. Passes the rows to the picker binary via stdin.
  4. On selection, calls "grove session open <name>" to create/attach the session.
  5. On cancel (Esc / Ctrl-C), exits silently with code 0.

This command is designed to be bound to a tmux key in your tmux.conf:
  bind-key S run-shell "grove session pick"`,
	Example: `  # Open the session picker (inside tmux — opens as a floating popup):
  grove session pick

  # Open the picker outside tmux (inline fzf):
  GROVE_HOME_ROOT=~/vault grove session pick

  # Typical tmux.conf binding:
  bind-key S run-shell "grove session pick"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionPick()
	},
}

func init() {
	sessionListCmd.Flags().BoolVar(&sessionListJSON, "json", false, "emit output as a JSON array")
	sessionCmd.AddCommand(sessionOpenCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionDeleteCmd)
	sessionCmd.AddCommand(sessionPickCmd)
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

type sessionListEntry struct {
	Name      string       `json:"name"`
	Tier      string       `json:"tier"`
	LaneCount int          `json:"lane_count"`
	Session   bool         `json:"session"`
	HomeDir   string       `json:"home_dir"`
	Lanes     []model.Lane `json:"lanes"`
}

func runSessionList(asJSON bool) error {
	tiers := []struct {
		dir   string
		label string
	}{
		{"01-Projects", "project"},
		{"02-Areas", "area"},
	}

	var entries []sessionListEntry
	maxNameLen := len("name")

	for _, tier := range tiers {
		tierDir := filepath.Join(cfg.HomeRoot, tier.dir)
		dirEntries, err := os.ReadDir(tierDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("reading vault tier %s: %w", tierDir, err)
		}
		for _, e := range dirEntries {
			if !e.IsDir() || isHiddenName(e.Name()) {
				continue
			}
			name := e.Name()
			homeDir := filepath.Join(tierDir, name)

			lanes := make([]model.Lane, 0)
			codeContainer := filepath.Join(cfg.CodeRoot, name)
			if _, statErr := os.Stat(codeContainer); statErr == nil {
				scanned, scanErr := scanner.ScanSourceSet(model.SourceSet{codeContainer})
				if scanErr != nil {
					return fmt.Errorf("scanning %s: %w", codeContainer, scanErr)
				}
				lanes = append(lanes, scanned...)
			}

			live, sessErr := tmux.HasSession(name)
			if sessErr != nil {
				slog.Debug("tmux.HasSession error", "name", name, "err", sessErr)
				live = false
			}

			entries = append(entries, sessionListEntry{
				Name:      name,
				Tier:      tier.label,
				LaneCount: len(lanes),
				Session:   live,
				HomeDir:   homeDir,
				Lanes:     lanes,
			})
			if len(name) > maxNameLen {
				maxNameLen = len(name)
			}
		}
	}

	if asJSON {
		// Rebuild as []model.Context for backwards-compatible JSON shape.
		contexts := make([]model.Context, len(entries))
		for i, e := range entries {
			contexts[i] = model.Context{
				Name:      e.Name,
				HomeDir:   e.HomeDir,
				SourceSet: model.SourceSet{filepath.Join(cfg.CodeRoot, e.Name)},
				Lanes:     e.Lanes,
			}
		}
		out, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling contexts: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	// Detect the active tmux session so we can mark it with *.
	var curSession string
	if os.Getenv("TMUX") != "" {
		curSession, _ = tmux.CurrentSessionName()
	}

	// Human-readable aligned table. Leading 2-char marker column ("* " or "  ").
	colName := maxNameLen
	colTier := len("project")
	colLanes := len("lanes")
	colSess := len("session")

	fmt.Printf("  %-*s  %-*s  %-*s  %-*s\n", colName, "name", colTier, "tier", colLanes, "lanes", colSess, "session")
	fmt.Printf("  %-*s  %-*s  %-*s  %-*s\n",
		colName, dashes(colName),
		colTier, dashes(colTier),
		colLanes, dashes(colLanes),
		colSess, dashes(colSess),
	)
	for _, e := range entries {
		marker := "  "
		if curSession != "" && e.Name == curSession {
			marker = "* "
		}
		fmt.Printf("%s%-*s  %-*s  %-*d  %-*s\n",
			marker,
			colName, e.Name,
			colTier, e.Tier,
			colLanes, e.LaneCount,
			colSess, boolStr(e.Session),
		)
	}
	return nil
}

// buildSessionPickRows scans vault tiers and returns fzf-ready rows plus the
// maximum context name length (used to size the popup).
//
// Row format (space-aligned):
//
//	coding-project-big    project  5 lanes  *
//	coding-project-small  project  2 lanes
//	health                area     0 lanes
//
// The trailing "*" appears when a live tmux session with that name exists.
func buildSessionPickRows() (rows []string, maxNameLen int, err error) {
	tiers := []struct {
		dir   string
		label string
	}{
		{"01-Projects", "project"},
		{"02-Areas", "area"},
	}

	type rowEntry struct {
		name      string
		tier      string
		laneCount int
		live      bool
	}

	var entries []rowEntry
	maxNameLen = 0

	for _, tier := range tiers {
		tierDir := filepath.Join(cfg.HomeRoot, tier.dir)
		dirEntries, readErr := os.ReadDir(tierDir)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			return nil, 0, fmt.Errorf("reading vault tier %s: %w", tierDir, readErr)
		}
		for _, e := range dirEntries {
			if !e.IsDir() || isHiddenName(e.Name()) {
				continue
			}
			name := e.Name()

			laneCount := 0
			codeContainer := filepath.Join(cfg.CodeRoot, name)
			if _, statErr := os.Stat(codeContainer); statErr == nil {
				scanned, scanErr := scanner.ScanSourceSet(model.SourceSet{codeContainer})
				if scanErr != nil {
					return nil, 0, fmt.Errorf("scanning %s: %w", codeContainer, scanErr)
				}
				laneCount = len(scanned)
			}

			live, sessErr := tmux.HasSession(name)
			if sessErr != nil {
				slog.Debug("tmux.HasSession error", "name", name, "err", sessErr)
				live = false
			}

			entries = append(entries, rowEntry{
				name:      name,
				tier:      tier.label,
				laneCount: laneCount,
				live:      live,
			})
			if len(name) > maxNameLen {
				maxNameLen = len(name)
			}
		}
	}

	rows = make([]string, 0, len(entries))
	for _, e := range entries {
		laneWord := "lanes"
		if e.laneCount == 1 {
			laneWord = "lane"
		}
		// Tab-delimited: name + TAB + rest. Fzf renders tabs as spaces so the
		// display looks aligned, but splitting on \t always recovers the exact
		// name even when it contains spaces (e.g. "AI Eng Job Hunt").
		row := fmt.Sprintf("%s\t%-7s  %d %s",
			e.name,
			e.tier,
			e.laneCount, laneWord,
		)
		if e.live {
			row += "  *"
		}
		rows = append(rows, row)
	}
	return rows, maxNameLen, nil
}

func runSessionPick() error {
	// When inside tmux and not already running inside a popup, re-invoke self
	// as a tmux display-popup so the picker floats over the current window.
	if os.Getenv("TMUX") != "" && os.Getenv("GROVE_POPUP_ACTIVE") != "1" {
		// We need the row count to size the popup, so we build rows twice:
		// once here to compute dimensions, then again inside the popup.
		rows, maxNameLen, err := buildSessionPickRows()
		if err != nil {
			return err
		}

		// Width: max name len + tier (7) + lane count col + padding + borders.
		// A typical row looks like: "<name>  project  5 lanes  *"
		// That's maxNameLen + 2 + 7 + 2 + ~10 chars = maxNameLen + ~21.
		// Add 6 for fzf chrome.
		width := maxNameLen + 27
		if width < 40 {
			width = 40
		}
		height := len(rows) + 6
		if height > 30 {
			height = 30
		}
		if height < 12 {
			height = 12
		}

		self, selfErr := os.Executable()
		if selfErr != nil {
			return fmt.Errorf("resolving executable path: %w", selfErr)
		}
		cmd := exec.Command("tmux", "display-popup",
			"-w", strconv.Itoa(width),
			"-h", strconv.Itoa(height+2),
			"-e", "GROVE_POPUP_ACTIVE=1",
			"-e", "GROVE_HOME_ROOT="+cfg.HomeRoot,
			"-e", "GROVE_CODE_ROOT="+cfg.CodeRoot,
			"-E", self+" session pick",
		)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Inside popup (or outside tmux): build rows and run the picker.
	rows, _, err := buildSessionPickRows()
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

	// Name is the first tab-delimited field — safe for names containing spaces.
	name := strings.SplitN(chosen, "\t", 2)[0]
	return runSessionOpen(name)
}

// isHiddenName reports whether a directory name starts with a dot.
func isHiddenName(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

func runSessionDelete(name string) error {
	exists, err := tmux.HasSession(name)
	if err != nil {
		return fmt.Errorf("checking session %q: %w", name, err)
	}
	if !exists {
		return fmt.Errorf("session %s not found", name)
	}
	if err := tmux.KillSession(name); err != nil {
		return fmt.Errorf("killing session %q: %w", name, err)
	}
	fmt.Printf("deleted session %s\n", name)
	return nil
}
