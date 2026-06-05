// Package tmux is a thin wrapper over the tmux CLI.
// Grove drives tmux by shelling out — no control-mode, no library binding.
// This keeps the integration durable and easy to trace.
package tmux

import (
	"errors"
	"fmt"
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
