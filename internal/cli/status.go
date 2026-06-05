package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/git"
	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

var (
	statusShort bool
	statusJSON  bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print current grove context status",
	Long: `Print the current grove context — session, window, repo, branch, working
directory, and base repo (if in a linked worktree).

Preconditions:
  - Designed to run inside tmux. If TMUX is not set, exits 0 with no output.

Output modes:
  default     — aligned human-readable table
  --short     — compact single-line suitable for tmux status-right
  --json      — JSON object with all fields

Fields:
  session   tmux session name
  window    tmux window name (full display string)
  repo      repository name (empty for the "home" window)
  branch    current branch (empty for "home")
  dir       absolute path to the active pane's working directory
  base_repo absolute path to the primary clone when in a linked worktree

Example tmux.conf:
  set -g status-right "#(grove status --short)"`,
	Example: `  # Human-readable table (inside tmux):
  grove status

  # Compact single-line for tmux status-right:
  grove status --short

  # Machine-readable JSON:
  grove status --json

  # Typical tmux.conf usage:
  set -g status-right "#(grove status --short)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(statusShort, statusJSON)
	},
}

func init() {
	statusCmd.Flags().BoolVar(&statusShort, "short", false, "compact single-line output for tmux status-right")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "emit output as a JSON object")
	rootCmd.AddCommand(statusCmd)
}

type statusInfo struct {
	Session  string `json:"session"`
	Window   string `json:"window"`
	Repo     string `json:"repo,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Dir      string `json:"dir,omitempty"`
	BaseRepo string `json:"base_repo,omitempty"`
}

func runStatus(short, asJSON bool) error {
	if os.Getenv("TMUX") == "" {
		return nil
	}

	session, err := tmux.CurrentSessionName()
	if err != nil {
		return fmt.Errorf("getting session name: %w", err)
	}

	window, err := tmux.CurrentWindowName()
	if err != nil {
		return fmt.Errorf("getting window name: %w", err)
	}

	repo, branch, isHome := model.ParseWindowName(window)

	dir, _ := tmux.CurrentPaneDir()

	var baseRepo string
	if dir != "" && git.IsRepo(dir) {
		if isWorktree, wtErr := git.IsWorktree(dir); wtErr == nil && isWorktree {
			if commonDir, cdErr := git.CommonDir(dir); cdErr == nil && filepath.IsAbs(commonDir) {
				baseRepo = filepath.Dir(commonDir)
			}
		}
	}

	info := statusInfo{
		Session:  session,
		Window:   window,
		Repo:     repo,
		Branch:   branch,
		Dir:      dir,
		BaseRepo: baseRepo,
	}
	if isHome {
		info.Repo = ""
		info.Branch = ""
	}

	if asJSON {
		out, jsonErr := json.MarshalIndent(info, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf("marshalling status: %w", jsonErr)
		}
		fmt.Println(string(out))
		return nil
	}

	if short {
		switch {
		case isHome:
			fmt.Printf("%s › home\n", session)
		case branch != "":
			fmt.Printf("%s › %s · %s\n", session, repo, branch)
		default:
			fmt.Printf("%s › %s\n", session, window)
		}
		return nil
	}

	// Human-readable aligned table.
	fmt.Printf("session  %s\n", session)
	fmt.Printf("window   %s\n", window)
	if !isHome {
		fmt.Printf("repo     %s\n", repo)
		fmt.Printf("branch   %s\n", branch)
	}
	if dir != "" {
		fmt.Printf("dir      %s\n", dir)
	}
	if baseRepo != "" {
		fmt.Printf("base     %s\n", baseRepo)
	}
	return nil
}
