package cli

import "github.com/spf13/cobra"

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage grove sessions",
}

var sessionOpenCmd = &cobra.Command{
	Use:   "open <context>",
	Short: "Open or attach to a context session (idempotent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionOpen(args[0])
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available contexts as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionList()
	},
}

func init() {
	sessionCmd.AddCommand(sessionOpenCmd)
	sessionCmd.AddCommand(sessionListCmd)
	rootCmd.AddCommand(sessionCmd)
}

func runSessionOpen(name string) error {
	// TODO(build-order-3): scan source set, build session idempotently, attach
	return nil
}

func runSessionList() error {
	// TODO(build-order-2): scan vault and code root, emit JSON contexts
	return nil
}
