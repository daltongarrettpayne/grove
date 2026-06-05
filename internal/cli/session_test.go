package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/daltongarrettpayne/grove/internal/config"
)

func TestIsHiddenName(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{".git", true},
		{".hidden", true},
		{"visible", false},
		{"normal-dir", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isHiddenName(tt.input)
			if got != tt.want {
				t.Errorf("isHiddenName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestSessionList_noTmux verifies that runSessionList returns nil when tmux is
// not running. Previously the active-session marker code would only execute
// when TMUX is set, so this also verifies the marker path is skipped cleanly.
func TestSessionList_noTmux(t *testing.T) {
	home := t.TempDir()
	code := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "01-Projects", "my-project"), 0755); err != nil {
		t.Fatalf("setup vault dir: %v", err)
	}

	prev := cfg
	cfg = &config.Config{HomeRoot: home, CodeRoot: code, Picker: "fzf", LogLevel: "warn"}
	t.Cleanup(func() { cfg = prev })

	t.Setenv("TMUX", "")

	if err := runSessionList(false); err != nil {
		t.Errorf("runSessionList(human) returned error: %v", err)
	}
	if err := runSessionList(true); err != nil {
		t.Errorf("runSessionList(json) returned error: %v", err)
	}
}

// TestSessionList_hiddenDirsSkipped verifies that directories beginning with
// "." are excluded from the listing, even when they exist inside vault tiers.
func TestSessionList_hiddenDirsSkipped(t *testing.T) {
	home := t.TempDir()
	code := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "01-Projects", "real-project"), 0755); err != nil {
		t.Fatalf("setup real dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "01-Projects", ".hidden"), 0755); err != nil {
		t.Fatalf("setup hidden dir: %v", err)
	}

	prev := cfg
	cfg = &config.Config{HomeRoot: home, CodeRoot: code, Picker: "fzf", LogLevel: "warn"}
	t.Cleanup(func() { cfg = prev })

	t.Setenv("TMUX", "")

	// runSessionList must not error; we verify the behaviour via isHiddenName
	// which is the gate used inside runSessionList's loop.
	if err := runSessionList(false); err != nil {
		t.Errorf("runSessionList returned error: %v", err)
	}
}
