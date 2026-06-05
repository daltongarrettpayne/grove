// Package git is a thin wrapper over the git CLI.
// Grove reads git state by shelling out — no libgit2, no go-git binding.
package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// run executes a git command in dir and returns trimmed stdout.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s in %q: %w", strings.Join(args, " "), dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRepo reports whether dir is a git working tree of any kind.
func IsRepo(dir string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Run() == nil
}

// CurrentBranch returns the current branch name in dir.
// Returns an empty string when HEAD is detached (no branch).
func CurrentBranch(dir string) (string, error) {
	return run(dir, "branch", "--show-current")
}

// CommonDir returns the path to the .git directory of the main repo.
// For a linked worktree git returns an absolute path; for the main checkout
// it returns the relative ".git". This difference is what IsWorktree uses.
func CommonDir(dir string) (string, error) {
	return run(dir, "rev-parse", "--git-common-dir")
}

// IsWorktree reports whether dir is a linked worktree (not the primary clone).
// It compares --git-dir (per-worktree) with --git-common-dir (shared).
func IsWorktree(dir string) (bool, error) {
	gitDir, err := run(dir, "rev-parse", "--git-dir")
	if err != nil {
		return false, err
	}
	commonDir, err := CommonDir(dir)
	if err != nil {
		return false, err
	}
	return gitDir != commonDir, nil
}

// RepoName returns the repository name for a working tree.
//
// Resolution order:
//  1. Remote origin URL (strips path components and .git suffix)
//  2. For linked worktrees: directory name of the main repo (via --git-common-dir)
//  3. Directory name of dir itself
func RepoName(dir string) (string, error) {
	out, err := run(dir, "remote", "get-url", "origin")
	if err == nil && out != "" {
		out = strings.TrimSuffix(out, ".git")
		// Split on / and : to handle both https and ssh remote URLs.
		parts := strings.FieldsFunc(out, func(r rune) bool { return r == '/' || r == ':' })
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// For a linked worktree --git-common-dir is an absolute path to the main
	// repo's .git directory; the repo name is its parent directory's base name.
	commonDir, err := CommonDir(dir)
	if err == nil && filepath.IsAbs(commonDir) {
		return filepath.Base(filepath.Dir(commonDir)), nil
	}

	return filepath.Base(dir), nil
}
