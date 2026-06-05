package cli

import (
	"testing"
)

func TestWindowDelete_notInTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	err := runWindowDelete("some-window")
	if err == nil {
		t.Fatal("expected error when TMUX is not set, got nil")
	}
}

func TestWindowDeleteCurrent_notInTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	err := runWindowDeleteCurrent()
	if err == nil {
		t.Fatal("expected error when TMUX is not set, got nil")
	}
}

// TestWindowDelete_homeGuard verifies that "home" cannot be deleted even when
// TMUX is set. The home guard fires before any tmux shell-out, so this test
// works without a running tmux server.
func TestWindowDelete_homeGuard(t *testing.T) {
	t.Setenv("TMUX", "/tmp/fake.sock,0,0")
	err := runWindowDelete("home")
	if err == nil {
		t.Fatal("expected error when deleting home window, got nil")
	}
	const want = "cannot delete the home window"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
