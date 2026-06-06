package cli

import (
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
		slog.Error("window pick: getting current session name", "err", err)
		return fmt.Errorf("getting current session name: %w", err)
	}

	// Fetch the real tmux window names — these are the authoritative targets
	// for SelectWindow. Using disk-derived rows (buildWindowRows) would produce
	// padded display strings like "kalashnikov-core    ·  main" that don't
	// match the actual tmux window names and break selection.
	names, err := tmux.ListWindows(sessionName)
	if err != nil {
		slog.Error("window pick: listing windows", "session", sessionName, "err", err)
		return fmt.Errorf("listing windows in session %q: %w", sessionName, err)
	}

	slog.Debug("window pick: got window names", "session", sessionName, "count", len(names))

	// When inside tmux and not already in a popup, re-invoke self inside a
	// tmux display-popup so the picker floats over the current window.
	if os.Getenv("GROVE_POPUP_ACTIVE") != "1" {
		maxNameLen := 0
		for _, n := range names {
			if len(n) > maxNameLen {
				maxNameLen = len(n)
			}
		}
		width := maxNameLen + 6
		height := len(names) + 5
		if height < 8 {
			height = 8
		}

		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolving executable path: %w", err)
		}
		popupArgs := append(tmux.SocketArgs(), "display-popup",
			"-w", strconv.Itoa(width),
			"-h", strconv.Itoa(height),
			"-e", "GROVE_POPUP_ACTIVE=1",
			"-e", "GROVE_HOME_ROOT="+cfg.HomeRoot,
			"-e", "GROVE_CODE_ROOT="+cfg.CodeRoot,
		)
		if cfg.TmuxSocket != "" {
			popupArgs = append(popupArgs, "-e", "GROVE_TMUX_SOCKET="+cfg.TmuxSocket)
		}
		popupArgs = append(popupArgs, "-E", self+" window pick")
		cmd := exec.Command("tmux", popupArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Ignore the display-popup exit code. When the user selects a window,
		// select-window closes the popup mid-execution, which kills the inner grove
		// process and causes display-popup to exit non-zero. That is the successful
		// case. Any genuine error was already shown inside the popup terminal.
		_ = cmd.Run()
		return nil
	}

	chosen, err := picker.NewFzf(cfg.Picker).Select(names)
	if err != nil {
		if errors.Is(err, picker.ErrCancelled) {
			slog.Debug("window pick: cancelled", "session", sessionName)
			return nil
		}
		slog.Error("window pick: picker error", "session", sessionName, "err", err)
		return fmt.Errorf("picker: %w", err)
	}

	slog.Info("window pick: selecting window", "session", sessionName, "window", chosen)
	if err := tmux.SelectWindow(sessionName, chosen); err != nil {
		slog.Error("window pick: SelectWindow failed", "session", sessionName, "window", chosen, "err", err)
		return fmt.Errorf("selecting window: %w", err)
	}
	return nil
}

func runWindowDeleteCurrent() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("grove window delete must be run inside a tmux session")
	}
	// kill-window with no -t kills the current window.
	if err := tmux.KillCurrentWindow(); err != nil {
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

	windows, err := tmux.ListWindows(sessionName)
	if err != nil {
		return fmt.Errorf("listing windows in session %q: %w", sessionName, err)
	}
	found := false
	for _, w := range windows {
		if w == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("window %q not found in session %q\nAvailable windows:\n  %s",
			name, sessionName, strings.Join(windows, "\n  "))
	}

	if err := tmux.KillWindow(sessionName, name); err != nil {
		return fmt.Errorf("killing window %q: %w", name, err)
	}
	fmt.Printf("deleted window %q\n", name)
	return nil
}
