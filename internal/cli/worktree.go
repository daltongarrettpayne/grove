package cli

import "github.com/spf13/cobra"

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees",
}

var worktreeRepo string

var worktreeNewCmd = &cobra.Command{
	Use:   "new <branch>",
	Short: "Create a linked worktree and register it as a lane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWorktreeNew(args[0], worktreeRepo)
	},
}

func init() {
	worktreeNewCmd.Flags().StringVar(&worktreeRepo, "repo", "", "path to the main repo (defaults to cwd)")
	worktreeCmd.AddCommand(worktreeNewCmd)
	rootCmd.AddCommand(worktreeCmd)
}

func runWorktreeNew(branch, repo string) error {
	// TODO(build-order-4): create worktree dir, git worktree add, set @pinned_name
	return nil
}
