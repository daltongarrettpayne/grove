package cli

import (
	"testing"
)

// TestStatus_outsideTmux verifies that runStatus exits 0 with no output when
// TMUX is not set — the contract for tmux status-right callers that may run
// the command before tmux is fully available.
func TestStatus_outsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	for _, tc := range []struct {
		short bool
		json  bool
	}{
		{false, false},
		{true, false},
		{false, true},
	} {
		if err := runStatus(tc.short, tc.json); err != nil {
			t.Errorf("runStatus(short=%v, json=%v) outside tmux returned error: %v",
				tc.short, tc.json, err)
		}
	}
}
