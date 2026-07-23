package cli

import "github.com/spf13/cobra"

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <env>",
		Short: "Decrypt, edit in $EDITOR, re-encrypt, and push",
		Long: `Opens the environment in your editor via a locked decrypt/re-encrypt
round-trip. Works on a dev machine over SSH and directly on the server —
this replaces hand-editing the live file.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M3")
		},
	}
	cmd.Flags().Bool("local", false, "operate on the local filesystem store (when running on the server)")
	return cmd
}
