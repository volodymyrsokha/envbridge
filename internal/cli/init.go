package cli

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a new project or join an existing one",
		Long: `With no .envbridge.yaml in the repository, runs the project wizard:
environments, hosts, store setup on each server, and import of existing
env files as the initial encrypted blobs.

With an existing .envbridge.yaml, sets up your identity and prints the
public key to add to the team's recipients list.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M5")
		},
	}
}
