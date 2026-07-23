package cli

import "github.com/spf13/cobra"

func newKeygenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keygen",
		Short: "Generate your age identity (~/.config/envbridge/identity.txt)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M2")
		},
	}
}
