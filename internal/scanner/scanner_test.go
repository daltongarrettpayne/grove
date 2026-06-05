package scanner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/scanner"
)

// initRepo creates a git repository at dir on the given branch with a single
// empty commit so that git branch --show-current returns a non-empty name.
func initRepo(t *testing.T, dir, branch string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "checkout", "-b", branch},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("setup: %s: %s", strings.Join(args, " "), out)
		}
	}
}

func TestScanSourceSet_empty(t *testing.T) {
	container := t.TempDir()
	lanes, err := scanner.ScanSourceSet(model.SourceSet{container})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lanes) != 0 {
		t.Errorf("expected empty slice, got %d lane(s)", len(lanes))
	}
}

func TestScanSourceSet_singleRepo(t *testing.T) {
	container := t.TempDir()
	repoDir := filepath.Join(container, "myrepo")

	initRepo(t, repoDir, "main")

	lanes, err := scanner.ScanSourceSet(model.SourceSet{container})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lanes) != 1 {
		t.Fatalf("expected 1 lane, got %d", len(lanes))
	}

	lane := lanes[0]
	if lane.Repo != "myrepo" {
		t.Errorf("Repo = %q, want %q", lane.Repo, "myrepo")
	}
	if lane.Branch != "main" {
		t.Errorf("Branch = %q, want %q", lane.Branch, "main")
	}
	if lane.Kind != model.LaneClone {
		t.Errorf("Kind = %v, want LaneClone", lane.Kind)
	}
}

func TestScanSourceSet_skipsNonGit(t *testing.T) {
	container := t.TempDir()
	// Create a plain directory — not a git repo.
	if err := os.Mkdir(filepath.Join(container, "notarepo"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	lanes, err := scanner.ScanSourceSet(model.SourceSet{container})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lanes) != 0 {
		t.Errorf("expected 0 lanes for non-git directory, got %d", len(lanes))
	}
}

func TestScanSourceSet_skipsHidden(t *testing.T) {
	container := t.TempDir()
	hiddenDir := filepath.Join(container, ".hidden")

	initRepo(t, hiddenDir, "main")

	lanes, err := scanner.ScanSourceSet(model.SourceSet{container})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lanes) != 0 {
		t.Errorf("expected 0 lanes for hidden directory, got %d", len(lanes))
	}
}

func TestScanSourceSet_multipleContainers(t *testing.T) {
	containerA := t.TempDir()
	containerB := t.TempDir()

	initRepo(t, filepath.Join(containerA, "alpha"), "main")
	initRepo(t, filepath.Join(containerB, "beta"), "develop")

	lanes, err := scanner.ScanSourceSet(model.SourceSet{containerA, containerB})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lanes) != 2 {
		t.Fatalf("expected 2 lanes, got %d", len(lanes))
	}

	// Results must be sorted by (repo, branch): alpha before beta.
	if lanes[0].Repo != "alpha" {
		t.Errorf("lanes[0].Repo = %q, want %q", lanes[0].Repo, "alpha")
	}
	if lanes[1].Repo != "beta" {
		t.Errorf("lanes[1].Repo = %q, want %q", lanes[1].Repo, "beta")
	}
}
