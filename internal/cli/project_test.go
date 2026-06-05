package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daltongarrettpayne/grove/internal/config"
)

// setupProjectCfg sets cfg to point at fresh temp directories and registers a
// cleanup that restores the previous cfg.
func setupProjectCfg(t *testing.T) (homeRoot, codeRoot string) {
	t.Helper()
	homeRoot = t.TempDir()
	codeRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(homeRoot, "01-Projects"), 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	prev := cfg
	cfg = &config.Config{HomeRoot: homeRoot, CodeRoot: codeRoot, Picker: "fzf", LogLevel: "warn"}
	t.Cleanup(func() { cfg = prev })
	return homeRoot, codeRoot
}

// TestProjectInit_vaultExistsCodeMissing is the core new behaviour: when
// --code is passed but the vault directory already exists and the code
// directory does not, project init should create the code directory and
// symlink rather than erroring.
func TestProjectInit_vaultExistsCodeMissing(t *testing.T) {
	homeRoot, codeRoot := setupProjectCfg(t)

	name := "my-project"
	vaultDir := filepath.Join(homeRoot, "01-Projects", name)
	codeDir := filepath.Join(codeRoot, name)
	symlink := filepath.Join(vaultDir, "code")

	// Pre-create the vault directory so vaultExists == true.
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatalf("pre-creating vault dir: %v", err)
	}

	// Run init with --code --no-git (avoids a real git init in the test).
	err := runProjectInit(name, true, true, "")
	if err != nil {
		t.Fatalf("runProjectInit returned error: %v", err)
	}

	// Code directory must have been created.
	if _, statErr := os.Stat(codeDir); statErr != nil {
		t.Errorf("code directory %s was not created: %v", codeDir, statErr)
	}

	// Symlink vault/code → codeDir must exist.
	if _, statErr := os.Lstat(symlink); statErr != nil {
		t.Errorf("symlink %s was not created: %v", symlink, statErr)
	}
	target, err := os.Readlink(symlink)
	if err != nil {
		t.Fatalf("reading symlink: %v", err)
	}
	if target != codeDir {
		t.Errorf("symlink target = %q, want %q", target, codeDir)
	}
}

// TestProjectInit_bothExist errors when both vault and code dirs already exist.
func TestProjectInit_bothExist(t *testing.T) {
	homeRoot, codeRoot := setupProjectCfg(t)

	name := "both-exist"
	vaultDir := filepath.Join(homeRoot, "01-Projects", name)
	codeDir := filepath.Join(codeRoot, name)

	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatalf("pre-creating vault dir: %v", err)
	}
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		t.Fatalf("pre-creating code dir: %v", err)
	}

	err := runProjectInit(name, true, true, "")
	if err == nil {
		t.Fatal("expected error when both vault and code dirs exist, got nil")
	}
	if !strings.Contains(err.Error(), "already fully initialized") {
		t.Errorf("error = %q, want it to mention \"already fully initialized\"", err.Error())
	}
}

// TestProjectInit_vaultExistsNoCodeFlag errors when the vault dir exists and
// --code is not passed, informing the user how to add a code directory.
func TestProjectInit_vaultExistsNoCodeFlag(t *testing.T) {
	homeRoot, _ := setupProjectCfg(t)

	name := "existing-vault"
	vaultDir := filepath.Join(homeRoot, "01-Projects", name)
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatalf("pre-creating vault dir: %v", err)
	}

	err := runProjectInit(name, false, false, "")
	if err == nil {
		t.Fatal("expected error when vault exists and --code not set, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to mention \"already exists\"", err.Error())
	}
}
