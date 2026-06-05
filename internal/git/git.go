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

// BranchExists reports whether the given branch name exists in dir.
func BranchExists(dir, branch string) (bool, error) {
	out, err := run(dir, "branch", "--list", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// WorktreeEntry is one entry from `git worktree list --porcelain`.
type WorktreeEntry struct {
	Dir    string // absolute path to the working tree
	Branch string // refs/heads/<name> stripped to just <name>; empty if detached
	IsMain bool   // true for the primary clone (first entry from git)
}

// ListWorktrees returns all worktrees for the repo containing repoDir.
func ListWorktrees(repoDir string) ([]WorktreeEntry, error) {
	out, err := run(repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}

	var entries []WorktreeEntry
	var current WorktreeEntry
	first := true

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			// Start of a new entry — save the previous one (if any).
			if current.Dir != "" {
				entries = append(entries, current)
			}
			current = WorktreeEntry{Dir: strings.TrimPrefix(line, "worktree ")}
			if first {
				current.IsMain = true
				first = false
			}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "detached":
			current.Branch = ""
		case line == "":
			// Blank line marks end of an entry block — nothing to do here;
			// we flush on the next "worktree " line or at the end.
		}
	}
	// Flush the last entry.
	if current.Dir != "" {
		entries = append(entries, current)
	}

	return entries, nil
}

// GetDefaultBranch returns the default branch name for the repo in dir.
// Resolution order: git symbolic-ref refs/remotes/origin/HEAD, then "main".
func GetDefaultBranch(dir string) (string, error) {
	out, err := run(dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil && out != "" {
		return strings.TrimPrefix(out, "refs/remotes/origin/"), nil
	}
	// Fallback: check if main exists remotely.
	if _, err2 := run(dir, "rev-parse", "--verify", "refs/remotes/origin/main"); err2 == nil {
		return "main", nil
	}
	return "main", nil
}

// GetMergedBranches returns local branch names that are fully merged into base.
// base itself is excluded from the result.
func GetMergedBranches(dir, base string) ([]string, error) {
	out, err := run(dir, "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("branch --merged: %w", err)
	}
	var branches []string
	for _, b := range strings.Split(out, "\n") {
		b = strings.TrimSpace(b)
		if b != "" && b != base {
			branches = append(branches, b)
		}
	}
	return branches, nil
}

// IsDirty reports whether dir has uncommitted changes (staged or unstaged).
func IsDirty(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// HasRemoteTracking reports whether branch in dir has a remote tracking ref.
func HasRemoteTracking(dir, branch string) (bool, error) {
	out, err := run(dir, "branch", "-vv", "--list", branch)
	if err != nil {
		return false, fmt.Errorf("branch -vv: %w", err)
	}
	return strings.Contains(out, "[origin/"), nil
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
