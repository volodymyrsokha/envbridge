package cli

import "github.com/spf13/cobra"

func newTeamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage recipients — who can decrypt the team's env files",
	}

	add := &cobra.Command{
		Use:   "add",
		Short: "Add a teammate's age public key to the recipients list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M5")
		},
	}
	add.Flags().String("name", "", "teammate's name")
	add.Flags().String("email", "", "teammate's email")
	add.Flags().String("key", "", "teammate's age public key")

	remove := &cobra.Command{
		Use:   "remove <name|email>",
		Short: "Remove a recipient and re-encrypt (rotate secrets they held!)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M5")
		},
	}

	sync := &cobra.Command{
		Use:   "sync [env...]",
		Short: "Re-encrypt environments for the current recipients list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M5")
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "Show the current recipients",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("M5")
		},
	}

	cmd.AddCommand(add, remove, sync, list)
	return cmd
}
