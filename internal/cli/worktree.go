package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

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
var worktreeListJSON bool

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

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all worktrees for the current (or specified) repo",
	Long: `List all worktrees for the repository.

Preconditions:
  - Must be run inside a git repository, or --repo must be provided.

What it does:
  1. Resolves the main repo by walking up from cwd (or uses --repo).
  2. Runs git worktree list --porcelain to enumerate worktrees.
  3. Prints an aligned table of branch and directory (human) or JSON array.
     The primary clone is marked "(main)".

Exits 0 on success. Exits non-zero if not inside a git repo.`,
	Example: `
  # List worktrees for the repo in cwd:
  grove worktree list

  # List as JSON:
  grove worktree list --json

  # List for a specific repo:
  grove worktree list --repo /path/to/repo`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWorktreeList(worktreeRepo, worktreeListJSON)
	},
}

var worktreeDeleteCmd = &cobra.Command{
	Use:   "delete <branch>",
	Short: "Remove a linked worktree (and its tmux window if present)",
	Args:  cobra.ExactArgs(1),
	Long: `Remove a linked worktree by branch name.

Preconditions:
  - Must be run inside a git repository, or --repo must be provided.
  - The branch must correspond to an existing linked worktree (not the main clone).

What it does:
  1. Resolves the main repo by walking up from cwd (or uses --repo).
  2. Finds the worktree entry matching the given branch name.
  3. Runs git worktree remove to delete it from disk.
  4. If running inside tmux, kills the matching window in the current session.

Exits 0 on success. Exits non-zero if the worktree is not found, is the main
clone, or git refuses to remove it (e.g. dirty working tree).`,
	Example: `
  # Delete the worktree for branch feat/user-auth:
  grove worktree delete feat/user-auth

  # Delete for a repo not in cwd:
  grove worktree delete feat/user-auth --repo /path/to/repo`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWorktreeDelete(args[0], worktreeRepo)
	},
}

func init() {
	worktreeNewCmd.Flags().StringVar(&worktreeRepo, "repo", "", "path to the main repo (defaults to cwd)")
	worktreeListCmd.Flags().StringVar(&worktreeRepo, "repo", "", "path to the main repo (defaults to cwd)")
	worktreeListCmd.Flags().BoolVar(&worktreeListJSON, "json", false, "output as JSON array")
	worktreeDeleteCmd.Flags().StringVar(&worktreeRepo, "repo", "", "path to the main repo (defaults to cwd)")
	worktreeCmd.AddCommand(worktreeNewCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeDeleteCmd)
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

// resolveMainRepo returns the absolute path of the main (non-linked) git repo.
// If repo is non-empty it is returned directly. Otherwise the function walks
// up from the current working directory until it finds a non-linked repo root.
func resolveMainRepo(repo string) (string, error) {
	if repo != "" {
		return repo, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	dir := cwd
	for {
		if git.IsRepo(dir) {
			isWT, err := git.IsWorktree(dir)
			if err != nil {
				return "", fmt.Errorf("checking worktree status of %q: %w", dir, err)
			}
			if !isWT {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("not inside a git repository (use --repo to specify)")
}

func runWorktreeNew(branch, repo string) error {
	// 1. Validate branch name before touching the filesystem.
	if err := model.ValidateBranchName(branch); err != nil {
		return err
	}

	// 2. Resolve the main repo.
	mainRepo, err := resolveMainRepo(repo)
	if err != nil {
		return err
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
		wid, err := tmux.NewWindow(sessionName, windowName, dest)
		if err != nil {
			return fmt.Errorf("creating tmux window: %w", err)
		}
		// Pin so shell prompt hooks leave grove's window name intact.
		if err := tmux.PinWindow(wid, windowName); err != nil {
			return fmt.Errorf("pinning tmux window: %w", err)
		}
		slog.Info("registered tmux window", "session", sessionName, "window", windowName)
	}

	return nil
}

func runWorktreeList(repo string, asJSON bool) error {
	mainRepo, err := resolveMainRepo(repo)
	if err != nil {
		return err
	}

	entries, err := git.ListWorktrees(mainRepo)
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}

	if asJSON {
		type jsonEntry struct {
			Branch string `json:"branch"`
			Dir    string `json:"dir"`
			IsMain bool   `json:"is_main"`
		}
		out := make([]jsonEntry, len(entries))
		for i, e := range entries {
			out[i] = jsonEntry{Branch: e.Branch, Dir: e.Dir, IsMain: e.IsMain}
		}
		b, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("marshalling worktrees: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		label := e.Branch
		if label == "" {
			label = "(detached)"
		}
		marker := ""
		if e.IsMain {
			marker = "(main)"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", label, e.Dir, marker); err != nil {
			return err
		}
	}
	return w.Flush()
}

func runWorktreeDelete(branch, repo string) error {
	mainRepo, err := resolveMainRepo(repo)
	if err != nil {
		return err
	}

	entries, err := git.ListWorktrees(mainRepo)
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}

	var target *git.WorktreeEntry
	for i := range entries {
		if entries[i].Branch == branch {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no worktree for branch %s", branch)
	}
	if target.IsMain {
		return fmt.Errorf("cannot delete the main worktree")
	}

	cmd := exec.Command("git", "-C", mainRepo, "worktree", "remove", target.Dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}

	fmt.Printf("removed worktree %s  (%s)\n", branch, target.Dir)

	// Kill the matching tmux window if inside tmux.
	if os.Getenv("TMUX") != "" {
		sessionName, err := tmux.CurrentSessionName()
		if err != nil {
			return fmt.Errorf("getting tmux session name: %w", err)
		}
		repoName, err := git.RepoName(mainRepo)
		if err != nil {
			return fmt.Errorf("getting repo name: %w", err)
		}
		windowName := repoName + "  ·  " + branch
		windows, err := tmux.ListWindows(sessionName)
		if err != nil {
			slog.Debug("could not list tmux windows, skipping window cleanup", "err", err)
		} else {
			for _, w := range windows {
				if w == windowName {
					if killErr := tmux.KillWindow(sessionName, windowName); killErr != nil {
						slog.Debug("could not kill tmux window", "window", windowName, "err", killErr)
					} else {
						slog.Info("killed tmux window", "session", sessionName, "window", windowName)
					}
					break
				}
			}
		}
	}

	return nil
}
