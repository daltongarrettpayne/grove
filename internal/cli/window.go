package cli

import "github.com/spf13/cobra"

var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Open the window picker for the current session",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWindowPicker()
	},
}

func init() {
	rootCmd.AddCommand(windowCmd)
}

func runWindowPicker() error {
	// TODO(build-order-5): scan source set, emit picker rows, switch on selection
	return nil
}
