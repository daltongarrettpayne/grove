// Command gen generates a deterministic synthetic world for testing grove.
//
// It creates a fake code root and vault tree that covers every context shape
// and worktree state grove needs to handle:
//
//	coding-project-big   — cockpit: two repos, various worktree states
//	coding-project-small — single-repo: main + WIP worktree
//	non-coding-project   — vault note only, no code
//	02-Areas/health      — area context (no project lifecycle)
//	04-Archive           — archived project (for doctor archived-context check)
//
// Worktree states covered:
//
//	clean / unmerged     — normal WIP (coding-project-small/repo)
//	dirty                — uncommitted changes (repo-beta, feat/dirty)
//	merged               — branch merged into main, worktree not removed (repo-alpha, feat/merged)
//	stale                — branch deleted from main repo, worktree remains (repo-alpha, feat/stale)
//	detached HEAD        — checked out to a commit, no branch (repo-beta, detached)
//
// Usage:
//
//	go run ./test/fixtures/gen                  # writes to /tmp/grove-fixtures
//	go run ./test/fixtures/gen -out /some/path  # custom output directory
//	go run ./test/fixtures/gen -grove bin/grove # explicit grove binary path
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
	out := flag.String("out", "/tmp/grove-fixtures", "output directory")
	groveBin := flag.String("grove", "bin/grove", "path to the grove binary")
	flag.Parse()

	grove, err := filepath.Abs(*groveBin)
	if err != nil {
		log.Fatalf("resolving grove binary: %v", err)
	}
	if _, err := os.Stat(grove); err != nil {
		log.Fatalf("grove binary not found at %s — run `make build` first", grove)
	}

	if err := generate(*out, grove); err != nil {
		log.Fatalf("fixture generation failed: %v", err)
	}

	fmt.Printf("fixtures written to %s\n\n", *out)
	fmt.Printf("To use these fixtures:\n")
	fmt.Printf("  export GROVE_CODE_ROOT=%s/code\n", *out)
	fmt.Printf("  export GROVE_HOME_ROOT=%s/vault\n", *out)
	fmt.Printf("\nTo inspect the world:\n")
	fmt.Printf("  grove project list\n")
	fmt.Printf("  grove session list\n")
	fmt.Printf("  grove doctor\n")
}

// ── top-level ──────────────────────────────────────────────────────────────

func generate(root, grove string) error {
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("removing old fixtures: %w", err)
	}

	vaultRoot := filepath.Join(root, "vault")
	codeRoot := filepath.Join(root, "code")

	// Vault tier scaffolding (grove project init needs 01-Projects to exist).
	for _, tier := range []string{"01-Projects", "02-Areas", "04-Archive", "04-Archive/01-Projects"} {
		if err := os.MkdirAll(filepath.Join(vaultRoot, tier), 0755); err != nil {
			return fmt.Errorf("creating vault tier %s: %w", tier, err)
		}
	}

	g := &gen{grove: grove, vaultRoot: vaultRoot, codeRoot: codeRoot}

	if err := g.buildProjects(); err != nil {
		return fmt.Errorf("building projects: %w", err)
	}
	if err := g.buildArea(); err != nil {
		return fmt.Errorf("building area: %w", err)
	}
	if err := g.buildArchivedProject(); err != nil {
		return fmt.Errorf("building archived project: %w", err)
	}
	return nil
}

// gen holds shared state for the generation run.
type gen struct {
	grove     string
	vaultRoot string
	codeRoot  string
}

// ── projects ───────────────────────────────────────────────────────────────

func (g *gen) buildProjects() error {
	if err := g.buildCockpit(); err != nil {
		return fmt.Errorf("cockpit: %w", err)
	}
	if err := g.buildSmallProject(); err != nil {
		return fmt.Errorf("small project: %w", err)
	}
	if err := g.buildNonCodingProject(); err != nil {
		return fmt.Errorf("non-coding project: %w", err)
	}
	return nil
}

// buildCockpit creates coding-project-big: two repos with all worktree states.
func (g *gen) buildCockpit() error {
	name := "coding-project-big"

	// Vault: grove project init (no --code; cockpit has multiple repos).
	if err := g.groveRun("project", "init", name); err != nil {
		return fmt.Errorf("project init: %w", err)
	}

	container := filepath.Join(g.codeRoot, name)
	if err := os.MkdirAll(container, 0755); err != nil {
		return err
	}

	// ── repo-alpha: clean main + merged worktree + stale worktree ──────────
	alphaMain := filepath.Join(container, "repo-alpha")
	if err := initRepo(alphaMain, "main"); err != nil {
		return fmt.Errorf("repo-alpha: %w", err)
	}

	// feat/merged: add commits, merge into main, leave worktree in place.
	// Directory uses the slugified name (feat/ prefix stripped).
	alphaWTMerged := filepath.Join(container, "repo-alpha-merged")
	if err := addWorktree(alphaMain, alphaWTMerged, "feat/merged"); err != nil {
		return err
	}
	if err := commitFile(alphaWTMerged, "merged.txt", "merged work"); err != nil {
		return err
	}
	if err := gitRun(alphaMain, "git", "merge", "--no-ff", "feat/merged", "-m", "merge feat/merged"); err != nil {
		return fmt.Errorf("merging feat/merged: %w", err)
	}

	// feat/stale: add worktree, then delete the branch from main repo using
	// git plumbing so the worktree directory remains but the branch is gone.
	alphaWTStale := filepath.Join(container, "repo-alpha-stale")
	if err := addWorktree(alphaMain, alphaWTStale, "feat/stale"); err != nil {
		return err
	}
	if err := gitRun(alphaMain, "git", "update-ref", "-d", "refs/heads/feat/stale"); err != nil {
		return fmt.Errorf("deleting stale branch ref: %w", err)
	}

	// ── repo-beta: clean main + dirty worktree + detached worktree ─────────
	betaMain := filepath.Join(container, "repo-beta")
	if err := initRepo(betaMain, "main"); err != nil {
		return fmt.Errorf("repo-beta: %w", err)
	}

	// feat/dirty: add worktree, write an unstaged file.
	betaWTDirty := filepath.Join(container, "repo-beta-dirty")
	if err := addWorktree(betaMain, betaWTDirty, "feat/dirty"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(betaWTDirty, "uncommitted.txt"), []byte("dirty work\n"), 0644); err != nil {
		return fmt.Errorf("writing dirty file: %w", err)
	}

	// detached: worktree with no branch (detached HEAD).
	betaWTDetached := filepath.Join(container, "repo-beta-detached")
	if err := gitRun(betaMain, "git", "worktree", "add", "--detach", betaWTDetached); err != nil {
		return fmt.Errorf("adding detached worktree: %w", err)
	}

	return nil
}

// buildSmallProject creates coding-project-small: single repo + clean WIP worktree.
func (g *gen) buildSmallProject() error {
	name := "coding-project-small"

	// Vault + code container (--no-git so we control git setup).
	if err := g.groveRun("project", "init", name, "--code", "--no-git"); err != nil {
		return fmt.Errorf("project init: %w", err)
	}

	// The code dir is the container. Init a repo inside it.
	container := filepath.Join(g.codeRoot, name)
	repoMain := filepath.Join(container, "repo")
	if err := initRepo(repoMain, "main"); err != nil {
		return fmt.Errorf("repo: %w", err)
	}

	// feat/landing: clean WIP — has commits but not merged, no dirty files.
	repoWTLanding := filepath.Join(container, "repo-landing")
	if err := addWorktree(repoMain, repoWTLanding, "feat/landing"); err != nil {
		return err
	}
	if err := commitFile(repoWTLanding, "landing.txt", "landing page WIP"); err != nil {
		return err
	}

	return nil
}

// buildNonCodingProject creates non-coding-project: vault note only.
func (g *gen) buildNonCodingProject() error {
	return g.groveRun("project", "init", "non-coding-project")
}

// ── area ───────────────────────────────────────────────────────────────────

// buildArea creates a 02-Areas/health entry. grove project init only targets
// 01-Projects, so this is created directly.
func (g *gen) buildArea() error {
	dir := filepath.Join(g.vaultRoot, "02-Areas", "health")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	content := "---\nname: health\ncreated: 2026-01-01\n---\n\n_Ongoing area._\n"
	return os.WriteFile(filepath.Join(dir, "context.md"), []byte(content), 0644)
}

// ── archive ────────────────────────────────────────────────────────────────

// buildArchivedProject creates an archived vault entry for doctor
// archived-context tests.
func (g *gen) buildArchivedProject() error {
	dir := filepath.Join(g.vaultRoot, "04-Archive", "01-Projects", "old-project")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	content := "---\nname: old-project\ncreated: 2025-01-01\n---\n\n_Archived._\n"
	return os.WriteFile(filepath.Join(dir, "context.md"), []byte(content), 0644)
}

// ── git helpers ────────────────────────────────────────────────────────────

// initRepo creates a new bare git repo at path on branch with one empty commit.
func initRepo(path, branch string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"git", "init", "-b", branch},
		{"git", "commit", "--allow-empty", "-m", "init"},
	} {
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

// commitFile writes content to filename in dir and commits it.
func commitFile(dir, filename, content string) error {
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content+"\n"), 0644); err != nil {
		return err
	}
	if err := gitRun(dir, "git", "add", filename); err != nil {
		return err
	}
	return gitRun(dir, "git", "commit", "-m", "add "+filename)
}

// gitRun executes a git command in dir with a fixture author identity so the
// generator works in a clean environment with no global git config.
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
		return fmt.Errorf("git %v in %s: %s: %w", args[1:], dir, string(out), err)
	}
	return nil
}

// ── grove helper ───────────────────────────────────────────────────────────

// groveRun calls the grove binary with GROVE_HOME_ROOT and GROVE_CODE_ROOT
// set to the fixture vault and code roots.
func (g *gen) groveRun(args ...string) error {
	cmd := exec.Command(g.grove, args...)
	cmd.Env = append(os.Environ(),
		"GROVE_HOME_ROOT="+g.vaultRoot,
		"GROVE_CODE_ROOT="+g.codeRoot,
		"GIT_AUTHOR_NAME=Grove Fixture",
		"GIT_AUTHOR_EMAIL=fixture@grove.test",
		"GIT_COMMITTER_NAME=Grove Fixture",
		"GIT_COMMITTER_EMAIL=fixture@grove.test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("grove %v: %s: %w", args, string(out), err)
	}
	return nil
}
