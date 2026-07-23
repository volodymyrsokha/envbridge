package cli

import "github.com/spf13/cobra"

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [env]",
		Short: "Show key-level differences between local and server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M4")
		},
	}
	cmd.Flags().Bool("show-values", false, "reveal secret values instead of masking them")
	cmd.Flags().Bool("server", false, "diff the canonical blob against the server's materialized file")
	return cmd
}
