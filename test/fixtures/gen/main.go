// Command gen generates a deterministic synthetic world for testing grove.
//
// It creates a fake code root and a fake vault tree that covers every context
// shape grove needs to handle:
//
//	coding-project-big   — two main repos, each with multiple worktrees
//	coding-project-small — one main repo plus a single worktree
//	non-coding-project   — no code directory (vault note only)
//
// The generated world doubles as the Docker clean-room fixture and the test
// harness fixture — same data, two uses.
//
// Usage:
//
//	go run ./test/fixtures/gen                     # writes to /tmp/grove-fixtures
//	go run ./test/fixtures/gen --out /some/path    # custom output directory
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	out := flag.String("out", "/tmp/grove-fixtures", "output directory for the fixture world")
	flag.Parse()

	if err := generate(*out); err != nil {
		log.Fatalf("fixture generation failed: %v", err)
	}

	fmt.Printf("fixtures written to %s\n", *out)
	fmt.Printf("  export GROVE_CODE_ROOT=%s/code\n", *out)
	fmt.Printf("  export GROVE_HOME_ROOT=%s/vault\n", *out)
}

func generate(root string) error {
	// Start clean every time so the fixture world is always deterministic.
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("removing old fixtures: %w", err)
	}

	codeRoot := filepath.Join(root, "code")
	vaultRoot := filepath.Join(root, "vault")

	if err := buildVault(vaultRoot); err != nil {
		return fmt.Errorf("building vault: %w", err)
	}
	if err := buildCode(codeRoot); err != nil {
		return fmt.Errorf("building code root: %w", err)
	}
	return nil
}

// buildVault creates the fake knowledge tree.
func buildVault(root string) error {
	dirs := []string{
		"01-Projects/coding-project-big",
		"01-Projects/coding-project-small",
		"01-Projects/non-coding-project",
		"02-Areas/health",
		"06-Meta/holmes-os",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			return err
		}
	}
	for _, proj := range []string{"coding-project-big", "coding-project-small", "non-coding-project"} {
		note := filepath.Join(root, "01-Projects", proj, "context.md")
		content := fmt.Sprintf("# %s\n\nfixture context note\n", proj)
		if err := os.WriteFile(note, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

// buildCode creates the fake code root with git repos and worktrees.
func buildCode(root string) error {
	// coding-project-big: two repos, each with multiple worktrees.
	bigContainer := filepath.Join(root, "coding-project-big")

	alphaMain := filepath.Join(bigContainer, "repo-alpha")
	if err := initRepo(alphaMain, "main"); err != nil {
		return fmt.Errorf("coding-project-big/repo-alpha: %w", err)
	}
	if err := addWorktree(alphaMain, filepath.Join(bigContainer, "repo-alpha-wt-one"), "feat/one"); err != nil {
		return fmt.Errorf("coding-project-big/repo-alpha-wt-one: %w", err)
	}
	if err := addWorktree(alphaMain, filepath.Join(bigContainer, "repo-alpha-wt-two"), "feat/two"); err != nil {
		return fmt.Errorf("coding-project-big/repo-alpha-wt-two: %w", err)
	}

	betaMain := filepath.Join(bigContainer, "repo-beta")
	if err := initRepo(betaMain, "main"); err != nil {
		return fmt.Errorf("coding-project-big/repo-beta: %w", err)
	}
	if err := addWorktree(betaMain, filepath.Join(bigContainer, "repo-beta-ui"), "feat/ui"); err != nil {
		return fmt.Errorf("coding-project-big/repo-beta-ui: %w", err)
	}

	// coding-project-small: one main repo plus a single worktree.
	smallContainer := filepath.Join(root, "coding-project-small")
	smallMain := filepath.Join(smallContainer, "repo")
	if err := initRepo(smallMain, "main"); err != nil {
		return fmt.Errorf("coding-project-small/repo: %w", err)
	}
	if err := addWorktree(smallMain, filepath.Join(smallContainer, "repo-feat"), "feat/landing"); err != nil {
		return fmt.Errorf("coding-project-small/repo-feat: %w", err)
	}

	// non-coding-project has no code directory — just the vault note above.
	return nil
}

// initRepo creates a new git repo at path with one empty commit on branch.
func initRepo(path, branch string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	steps := [][]string{
		{"git", "init", "-b", branch},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range steps {
		if err := gitRun(path, args...); err != nil {
			return err
		}
	}
	return nil
}

// addWorktree creates branch in mainRepo and adds a linked worktree at dest.
func addWorktree(mainRepo, dest, branch string) error {
	return gitRun(mainRepo, "git", "worktree", "add", dest, "-b", branch)
}

// gitRun executes a git command in dir, injecting a fixture author identity so
// the generator works in a clean Docker environment with no global git config.
func gitRun(dir string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Grove Fixture",
		"GIT_AUTHOR_EMAIL=fixture@grove.test",
		"GIT_COMMITTER_NAME=Grove Fixture",
		"GIT_COMMITTER_EMAIL=fixture@grove.test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s: %w", args, string(out), err)
	}
	return nil
}
