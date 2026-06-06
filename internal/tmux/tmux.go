// Package tmux is a thin wrapper over the tmux CLI.
// Grove drives tmux by shelling out — no control-mode, no library binding.
// This keeps the integration durable and easy to trace.
package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var (
	// ErrNotRunning means the tmux server is not running on this socket.
	ErrNotRunning = errors.New("tmux server is not running")
	// ErrSessionExists means a session with that name is already alive.
	ErrSessionExists = errors.New("session already exists")
)

// socket is the optional private tmux socket path. When set, every tmux
// invocation targets it via `-S` so grove addresses the same server whether or
// not it is run from inside a tmux session.
var socket string

// SetSocket configures the tmux socket grove talks to. Empty means use tmux's
// default socket. Call once at startup from the loaded config.
func SetSocket(s string) { socket = s }

// SocketArgs returns the `-S <socket>` prefix when a private socket is
// configured, or nil. Exposed so callers that build their own tmux command
// line (e.g. display-popup) target the same server as the rest of grove.
func SocketArgs() []string {
	if socket == "" {
		return nil
	}
	return []string{"-S", socket}
}

// command builds an *exec.Cmd for tmux with the socket prefix applied.
func command(args ...string) *exec.Cmd {
	return exec.Command("tmux", append(SocketArgs(), args...)...)
}

// run executes a tmux command and returns trimmed stdout.
func run(args ...string) (string, error) {
	cmd := command(args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// HasSession reports whether a tmux session with the given name is running.
func HasSession(name string) (bool, error) {
	err := command("has-session", "-t", name).Run()
	if err == nil {
		return true, nil
	}
	// Exit code 1 means the session does not exist — that is not a program error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("tmux has-session: %w", err)
}

// NewSession creates a new detached session named `name` rooted at `dir`, with
// its initial window named `windowName`.
//
// The window is named at creation time rather than by renaming index 0
// afterward, because the user's `base-index` may not be 0: with
// `set -g base-index 1` the first window is index 1, so a `rename-window -t
// name:0` would fail and abort session build. Naming via `-n` is index-agnostic.
func NewSession(name, dir, windowName string) (string, error) {
	id, err := run("new-session", "-d", "-s", name, "-n", windowName, "-c", dir, "-P", "-F", "#{window_id}")
	if err != nil {
		return "", fmt.Errorf("creating session %q: %w", name, err)
	}
	return id, nil
}

// NewWindow creates a window named `name` in `session`, with its cwd set to
// `dir`, and returns the new window's stable id (e.g. "@5").
func NewWindow(session, name, dir string) (string, error) {
	id, err := run("new-window", "-t", session, "-n", name, "-c", dir, "-P", "-F", "#{window_id}")
	if err != nil {
		return "", fmt.Errorf("creating window %q in %q: %w", name, session, err)
	}
	return id, nil
}

// PinWindow marks a window with the @pinned_name option so that shell prompt
// hooks which auto-rename windows leave grove's name intact. This is grove's
// side of a documented integration contract: a window-renaming hook checks
// @pinned_name and skips windows that have it set. The value is grove's window
// name; consumers only require it to be non-empty. windowID should be a stable
// window id ("@5") as returned by NewSession/NewWindow.
func PinWindow(windowID, name string) error {
	if _, err := run("set-option", "-t", windowID, "-w", "@pinned_name", name); err != nil {
		return fmt.Errorf("pinning window %q: %w", windowID, err)
	}
	return nil
}

// ListSessions returns the names of all running tmux sessions.
func ListSessions() ([]string, error) {
	out, err := run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// AttachOrSwitch attaches to session if we are outside tmux, or switches the
// client to it if we are already inside a tmux session ($TMUX is set).
func AttachOrSwitch(name string) error {
	if os.Getenv("TMUX") != "" {
		_, err := run("switch-client", "-t", name)
		if err != nil {
			return fmt.Errorf("switching to session %q: %w", name, err)
		}
		return nil
	}
	cmd := command("attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("attaching to session %q: %w", name, err)
	}
	return nil
}

// DisplayMessage runs `tmux display-message -p <format>` and returns the result.
// This is the standard way to read tmux state (session name, window name, etc.).
func DisplayMessage(format string) (string, error) {
	out, err := run("display-message", "-p", format)
	if err != nil {
		return "", fmt.Errorf("display-message %q: %w", format, err)
	}
	return out, nil
}

// CurrentSessionName returns the name of the currently attached tmux session.
func CurrentSessionName() (string, error) {
	return DisplayMessage("#{session_name}")
}

// CurrentWindowName returns the name of the currently active tmux window.
func CurrentWindowName() (string, error) {
	return DisplayMessage("#{window_name}")
}

// CurrentPaneDir returns the current working directory of the active pane.
func CurrentPaneDir() (string, error) {
	return DisplayMessage("#{pane_current_path}")
}

// SelectWindow switches the active window in session to the one matching target.
// target may be a window name or index: e.g. "home" or "mysession:2".
func SelectWindow(session, target string) error {
	_, err := run("select-window", "-t", session+":"+target)
	if err != nil {
		return fmt.Errorf("selecting window %q in %q: %w", target, session, err)
	}
	return nil
}

// KillSession kills the tmux session with the given name.
func KillSession(name string) error {
	_, err := run("kill-session", "-t", name)
	if err != nil {
		return fmt.Errorf("killing session %q: %w", name, err)
	}
	return nil
}

// KillCurrentWindow kills the window the calling pane belongs to (no -t).
// Intended for use from inside a tmux session.
func KillCurrentWindow() error {
	if err := command("kill-window").Run(); err != nil {
		return fmt.Errorf("tmux kill-window: %w", err)
	}
	return nil
}

// ListWindows returns the names of all windows in the given session.
func ListWindows(session string) ([]string, error) {
	out, err := run("list-windows", "-t", session, "-F", "#{window_name}")
	if err != nil {
		return nil, fmt.Errorf("listing windows in %q: %w", session, err)
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// KillWindow kills a window by name inside session.
// Returns nil if the window was not found (idempotent).
func KillWindow(session, windowName string) error {
	target := session + ":" + windowName
	err := command("kill-window", "-t", target).Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("tmux kill-window %q: %w", target, err)
}
