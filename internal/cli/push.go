package cli

import "github.com/spf13/cobra"

func newPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push [env...]",
		Short: "Encrypt and upload local env files",
		Long: `Pushes with full safety rails: per-env lock, conflict detection against
the last pulled version, hand-edit detection on the server, backup of the
previous version, atomic swap, and plaintext materialization for the app.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M4")
		},
	}
	cmd.Flags().Bool("force", false, "skip conflict and hand-edit checks")
	return cmd
}
