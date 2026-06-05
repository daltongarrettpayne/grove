// Package picker abstracts the interactive selection step.
// Any binary that reads newline-separated rows on stdin and writes the
// selected row on stdout satisfies the Backend interface: fzf, sk, fzy,
// television, gum — they all qualify. The user names the binary in config.
package picker

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrCancelled is returned when the user exits without making a selection
// (Escape, Ctrl-C, etc.). Callers should treat this as a no-op, not an error.
var ErrCancelled = errors.New("picker: selection cancelled")

// Backend is the interface every picker binary must satisfy.
type Backend interface {
	Select(rows []string) (string, error)
}

// Fzf is the default Backend, backed by the fzf binary.
type Fzf struct {
	Binary string   // name or absolute path of the fzf binary
	Args   []string // additional fzf flags appended to every Select call
}

// NewFzf returns a Fzf backend. If binary is empty, "fzf" is used.
// Any extra args are appended to the fzf command line on every Select call,
// making it easy to pass display flags like --height or --no-info without
// changing any existing callers that pass no extra args.
func NewFzf(binary string, args ...string) *Fzf {
	if binary == "" {
		binary = "fzf"
	}
	return &Fzf{Binary: binary, Args: args}
}

func (f *Fzf) Select(rows []string) (string, error) {
	cmdArgs := append([]string{}, f.Args...)
	cmd := exec.Command(f.Binary, cmdArgs...)
	cmd.Stdin = strings.NewReader(strings.Join(rows, "\n"))

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 130 {
			// fzf exits 130 when the user presses Escape or Ctrl-C.
			return "", ErrCancelled
		}
		return "", fmt.Errorf("%s: %w", f.Binary, err)
	}
	return strings.TrimSpace(string(out)), nil
}
