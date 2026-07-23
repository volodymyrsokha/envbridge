package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/agecrypt"
	"github.com/volodymyrsokha/envbridge/internal/config"
	"github.com/volodymyrsokha/envbridge/internal/ui"
)

func newKeygenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keygen",
		Short: "Generate your age identity",
		Long: `Creates your personal age keypair. The private key stays on this machine
(0600); the public key is what teammates add to the recipients list so
environments can be encrypted for you.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.IdentityPath()
			if err != nil {
				return err
			}
			id, err := agecrypt.GenerateIdentity(path)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, ui.Success("Identity created: "+path))
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Your public key:")
			fmt.Fprintln(out, "  "+ui.Emphasize(id.Recipient().String()))
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Next steps:")
			fmt.Fprintln(out, "  • joining a team?  add this key under recipients: in .envbridge.yaml (open a PR)")
			fmt.Fprintln(out, "  • starting fresh?  run `envbridge init`")
			return nil
		},
	}
}
