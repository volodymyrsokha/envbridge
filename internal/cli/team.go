package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/agecrypt"
	"github.com/volodymyrsokha/envbridge/internal/config"
	"github.com/volodymyrsokha/envbridge/internal/store"
	"github.com/volodymyrsokha/envbridge/internal/ui"
)

func newTeamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage recipients — who can decrypt the team's env files",
	}
	cmd.AddCommand(newTeamListCmd(), newTeamAddCmd(), newTeamRemoveCmd(), newTeamSyncCmd())
	return cmd
}

func projectFilePath() (string, error) {
	_, root, err := config.Discover(".")
	if err != nil {
		return "", err
	}
	return filepath.Join(root, config.ProjectFileName), nil
}

func newTeamListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the current recipients",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := config.Discover(".")
			if err != nil {
				return err
			}
			nameW, emailW := 0, 0
			for _, r := range cfg.Recipients {
				nameW = max(nameW, len(r.Name))
				emailW = max(emailW, len(r.Email))
			}
			for _, r := range cfg.Recipients {
				fmt.Printf("  %-*s  %-*s  %s\n", nameW, r.Name, emailW, r.Email, r.Key)
			}
			return nil
		},
	}
}

func newTeamAddCmd() *cobra.Command {
	var name, email, key string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a teammate's age public key to the recipients list",
		Long: `Adds a recipient to .envbridge.yaml (preserving your comments and
formatting) and offers to re-encrypt every environment for the new roster.
The teammate gets their key from ` + "`envbridge keygen`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := projectFilePath()
			if err != nil {
				return err
			}
			if name == "" {
				name = ui.AskRequired("Teammate's name?")
			}
			if email == "" {
				email = ui.AskRequired("Teammate's email?")
			}
			if key == "" {
				key = ui.AskRequired("Teammate's age public key (from `envbridge keygen`)?")
			}
			if _, err := agecrypt.ParseRecipient(key); err != nil {
				return err
			}
			if err := config.AddRecipient(path, config.Recipient{Name: name, Email: email, Key: key}); err != nil {
				return err
			}
			fmt.Println(ui.Success(name + " added to " + config.ProjectFileName + " — commit the change"))
			fmt.Println(ui.Hint("environments stay encrypted for the old roster until `envbridge team sync` runs"))
			if ui.Confirm("Run `envbridge team sync` now?") {
				return runTeamSync(cmd.Context(), nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "teammate's name")
	cmd.Flags().StringVar(&email, "email", "", "teammate's email")
	cmd.Flags().StringVar(&key, "key", "", "teammate's age public key")
	return cmd
}

func newTeamRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name|email>",
		Short: "Remove a recipient and re-encrypt (rotate secrets they held!)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := projectFilePath()
			if err != nil {
				return err
			}
			removed, err := config.RemoveRecipient(path, args[0])
			if err != nil {
				return err
			}
			fmt.Println(ui.Success(removed.Name + " removed from " + config.ProjectFileName + " — commit the change"))
			fmt.Println()
			fmt.Println("⚠ Re-encryption stops future reads only. " + removed.Name + " could have copied")
			fmt.Println("  every secret while they had access — also revoke their SSH access and")
			fmt.Println("  rotate any secrets they shouldn't retain.")
			fmt.Println()
			if ui.Confirm("Run `envbridge team sync` now?") {
				return runTeamSync(cmd.Context(), nil)
			}
			return nil
		},
	}
}

func newTeamSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync [env...]",
		Short: "Re-encrypt environments for the current recipients list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTeamSync(cmd.Context(), args)
		},
	}
}

func runTeamSync(ctx context.Context, args []string) error {
	p, err := openProject()
	if err != nil {
		return err
	}
	defer p.Close()
	envs, err := p.envList(args)
	if err != nil {
		return err
	}
	fingerprint := store.RecipientsFingerprint(p.cfg.RecipientKeys())
	for _, env := range envs {
		if err := syncOne(ctx, p, env, fingerprint); err != nil {
			return fmt.Errorf("%s: %w", env, err)
		}
	}
	return nil
}

func syncOne(ctx context.Context, p *project, env, fingerprint string) error {
	envCfg, err := p.envConfig(env)
	if err != nil {
		return err
	}
	sess, err := p.sessionFor(env)
	if err != nil {
		return err
	}
	unlock, err := sess.Store.Lock(ctx, env, store.LockInfo{Who: whoami(), Host: hostname(), At: time.Now().UTC()})
	if err != nil {
		return err
	}
	defer unlock()

	m, plaintext, err := sess.Current(ctx, env)
	if err != nil {
		return err
	}
	if m == nil {
		fmt.Println(ui.Hint(env + " is not in the store yet — nothing to re-encrypt"))
		return nil
	}
	if m.RecipientsFingerprint == fingerprint {
		fmt.Println(ui.Success(env + " already encrypted for the current roster"))
		return nil
	}
	newM, err := sess.Write(ctx, env, plaintext, envCfg.Materialize)
	if err != nil {
		return err
	}
	if err := p.saveBase(env, newM); err != nil {
		return err
	}
	fmt.Println(ui.Success(fmt.Sprintf("%s re-encrypted for %d recipient(s)", env, len(p.cfg.Recipients))))
	return nil
}
