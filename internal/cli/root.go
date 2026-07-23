// Package cli wires the cobra command tree. Commands stay thin: parse flags,
// call store/ops, render through ui.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/ui"
	"github.com/volodymyrsokha/envbridge/internal/version"
)

var (
	flagJSON    bool
	flagNoColor bool
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "envbridge",
		Short: "Team .env management over SSH — no middle server",
		Long: `envbridge syncs encrypted env files between your team and your own server
over the SSH access you already have.

Canonical env files live on your server, encrypted with age to your team's
public keys. envbridge materializes a plaintext .env for your app on every
push, detects hand-edits and conflicts, and keeps backups.`,
		Example: `  envbridge init                 # bootstrap a project or join one
  envbridge pull production      # fetch + decrypt to your local env file
  envbridge edit staging         # decrypt → $EDITOR → re-encrypt → push
  envbridge status               # sync state of every environment`,
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			if flagNoColor || os.Getenv("NO_COLOR") != "" {
				ui.DisableColor()
			}
		},
	}

	cmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable output")
	cmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable colored output")

	cmd.AddCommand(
		newInitCmd(),
		newPullCmd(),
		newPushCmd(),
		newDiffCmd(),
		newStatusCmd(),
		newEditCmd(),
		newTeamCmd(),
		newKeygenCmd(),
		newVersionCmd(),
	)
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("envbridge %s (%s)\n", version.Version, version.Commit)
		},
	}
}

func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, ui.RenderError(err))
		os.Exit(1)
	}
}

func notImplemented(milestone string) error {
	return fmt.Errorf("not implemented yet — planned for %s, see DESIGN.md", milestone)
}
