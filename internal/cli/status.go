package cli

import "github.com/spf13/cobra"

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the sync state of every environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M4")
		},
	}
}
