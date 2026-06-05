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

// run executes a tmux command and returns trimmed stdout.
func run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// HasSession reports whether a tmux session with the given name is running.
func HasSession(name string) (bool, error) {
	err := exec.Command("tmux", "has-session", "-t", name).Run()
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

// NewSession creates a new detached session named `name` rooted at `dir`.
func NewSession(name, dir string) error {
	_, err := run("new-session", "-d", "-s", name, "-c", dir)
	if err != nil {
		return fmt.Errorf("creating session %q: %w", name, err)
	}
	return nil
}

// NewWindow creates a window named `name` in `session`, with its cwd set to `dir`.
func NewWindow(session, name, dir string) error {
	_, err := run("new-window", "-t", session, "-n", name, "-c", dir)
	if err != nil {
		return fmt.Errorf("creating window %q in %q: %w", name, session, err)
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

// SetWindowOption sets a tmux option on the given window.
// Use this to persist metadata like @pinned_name so grove can identify
// windows after auto-rename runs.
func SetWindowOption(session, window, key, value string) error {
	target := session + ":" + window
	_, err := run("set-option", "-t", target, "-w", key, value)
	return err
}

// RenameWindow renames the window at session:index to name.
func RenameWindow(session, windowIndex, name string) error {
	target := session + ":" + windowIndex
	_, err := run("rename-window", "-t", target, name)
	if err != nil {
		return fmt.Errorf("renaming window %s: %w", target, err)
	}
	return nil
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
	cmd := exec.Command("tmux", "attach-session", "-t", name)
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

// SelectWindow switches the active window in session to the one matching target.
// target may be a window name or index: e.g. "home" or "mysession:2".
func SelectWindow(session, target string) error {
	_, err := run("select-window", "-t", session+":"+target)
	if err != nil {
		return fmt.Errorf("selecting window %q in %q: %w", target, session, err)
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

// KillSession kills the named tmux session.
// Returns nil if the session was not running (idempotent).
func KillSession(name string) error {
	err := exec.Command("tmux", "kill-session", "-t", name).Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("tmux kill-session %q: %w", name, err)
}

// KillWindow kills a window by name inside session.
// Returns nil if the window was not found (idempotent).
func KillWindow(session, windowName string) error {
	target := session + ":" + windowName
	err := exec.Command("tmux", "kill-window", "-t", target).Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("tmux kill-window %q: %w", target, err)
}
