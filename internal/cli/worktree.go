package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/git"
	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees",
	Long: `Commands for creating and managing git linked worktrees as grove lanes.

A grove worktree is a git linked worktree whose directory is named
"<repo>-<slug>" (where slug is the branch name with feat/fix/chore/refactor
prefix stripped, slashes and underscores replaced with hyphens). This naming
convention is what grove's scanner and doctor checks rely on.`,
}

var worktreeRepo string

var worktreeNewCmd = &cobra.Command{
	Use:   "new <branch>",
	Short: "Create a linked worktree and register it as a lane",
	Long: `Create a git linked worktree for <branch> and, if inside a tmux session,
add a new window for it in the current session.

Preconditions:
  - <branch> must follow the grove branch naming convention:
    "<type>/<description>" where type is one of: feat, fix, chore, refactor,
    docs, test, or just a plain kebab-case name (e.g. "main").
  - The command must be run from inside a git repository, or --repo must point
    to the main repo directory (not a linked worktree).
  - The computed destination directory must not already exist.

What it does:
  1. Validates <branch> against the naming convention.
  2. Resolves the main repo: walks up from cwd until it finds a non-worktree
     git repo, or uses --repo if given.
  3. Computes destination: <parent-of-repo>/<repo-name>-<slug>/
     (slug = branch with type prefix stripped, / and _ replaced with -)
  4. Runs "git worktree add <dest> -b <branch>" in the main repo.
  5. If inside tmux, creates a new window in the current session named
     "<repo>  ·  <branch>" rooted at <dest>.

Exit behaviour: exits 0 on success. Exits non-zero if branch validation fails,
the main repo cannot be found, the destination exists, or git fails.`,
	Example: `  # Create a worktree for a new feature branch (inside a grove session):
  grove worktree new feat/my-feature
  # Creates: ../grove-my-feature/ and a tmux window "grove  ·  feat/my-feature"

  # Create from outside tmux, specifying the repo explicitly:
  grove worktree new fix/null-check --repo ~/code/grove/grove

  # Plain branch name (no type prefix):
  grove worktree new spike-observability`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWorktreeNew(args[0], worktreeRepo)
	},
}

func init() {
	worktreeNewCmd.Flags().StringVar(&worktreeRepo, "repo", "", "path to the main repo (defaults to cwd)")
	worktreeCmd.AddCommand(worktreeNewCmd)
	rootCmd.AddCommand(worktreeCmd)
}

// slugifyBranch strips common prefixes (feat/, fix/, chore/, refactor/) from a
// branch name, then lowercases and replaces / and _ with -.
func slugifyBranch(branch string) string {
	prefixes := []string{"feat/", "fix/", "chore/", "refactor/"}
	for _, p := range prefixes {
		if strings.HasPrefix(branch, p) {
			branch = strings.TrimPrefix(branch, p)
			break
		}
	}
	branch = strings.ToLower(branch)
	branch = strings.ReplaceAll(branch, "/", "-")
	branch = strings.ReplaceAll(branch, "_", "-")
	return branch
}

func runWorktreeNew(branch, repo string) error {
	// 1. Validate branch name before touching the filesystem.
	if err := model.ValidateBranchName(branch); err != nil {
		return err
	}

	// 2. Resolve the main repo.
	var mainRepo string
	if repo != "" {
		mainRepo = repo
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		dir := cwd
		for {
			if git.IsRepo(dir) {
				isWT, err := git.IsWorktree(dir)
				if err != nil {
					return fmt.Errorf("checking worktree status of %q: %w", dir, err)
				}
				if !isWT {
					mainRepo = dir
					break
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				// Reached the filesystem root without finding a main repo.
				break
			}
			dir = parent
		}
		if mainRepo == "" {
			return errors.New("not inside a git repository (use --repo to specify)")
		}
	}

	slog.Debug("resolved main repo", "path", mainRepo)

	// 3. Compute the worktree destination directory.
	repoName := filepath.Base(mainRepo)
	slug := slugifyBranch(branch)
	dest := filepath.Join(filepath.Dir(mainRepo), repoName+"-"+slug)

	slog.Debug("worktree destination", "dest", dest)

	// 4. Create the worktree.
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}

	cmd := exec.Command("git", "-C", mainRepo, "worktree", "add", dest, "-b", branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	slog.Info("created worktree", "branch", branch, "dest", dest)

	// 5. Register as a tmux window (only if running inside tmux).
	if os.Getenv("TMUX") != "" {
		sessionName, err := tmux.CurrentSessionName()
		if err != nil {
			return fmt.Errorf("getting tmux session name: %w", err)
		}
		windowName := repoName + "  ·  " + branch
		if err := tmux.NewWindow(sessionName, windowName, dest); err != nil {
			return fmt.Errorf("creating tmux window: %w", err)
		}
		slog.Info("registered tmux window", "session", sessionName, "window", windowName)
	}

	return nil
}
