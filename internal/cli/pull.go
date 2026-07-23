package cli

import "github.com/spf13/cobra"

func newPullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull [env...]",
		Short: "Fetch, decrypt, and write local env files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M4")
		},
	}
	cmd.Flags().Bool("adopt", false, "adopt hand-edits found on the server as the new canonical version")
	cmd.Flags().Bool("force", false, "overwrite local changes without confirmation")
	return cmd
}
