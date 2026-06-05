package cli

import "github.com/spf13/cobra"

var statusSegmentCmd = &cobra.Command{
	Use:   "status-segment",
	Short: "Print the tmux status-right segment for the current window",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatusSegment()
	},
}

func init() {
	rootCmd.AddCommand(statusSegmentCmd)
}

func runStatusSegment() error {
	// TODO(build-order-6): emit formatted segment for tmux status-right
	return nil
}
