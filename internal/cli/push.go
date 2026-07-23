package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/envdiff"
	"github.com/volodymyrsokha/envbridge/internal/envfile"
	"github.com/volodymyrsokha/envbridge/internal/store"
	"github.com/volodymyrsokha/envbridge/internal/ui"
)

func newPushCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "push [env...]",
		Short: "Encrypt and upload local env files",
		Long: `Pushes with full safety rails: per-env lock, conflict detection against
the last pulled version, hand-edit detection on the server, backup of the
previous version, atomic swap, and plaintext materialization for the app.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := openProject()
			if err != nil {
				return err
			}
			defer p.Close()
			envs, err := p.envList(args)
			if err != nil {
				return err
			}
			for _, env := range envs {
				if err := pushOne(cmd.Context(), p, env, force); err != nil {
					return fmt.Errorf("%s: %w", env, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip conflict and hand-edit checks")
	return cmd
}

func pushOne(ctx context.Context, p *project, env string, force bool) error {
	envCfg, err := p.envConfig(env)
	if err != nil {
		return err
	}
	localPath := p.localPath(envCfg)
	data, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no local file %s — run `envbridge pull %s` first", envCfg.Local, env)
		}
		return err
	}
	if malformed := envfile.Parse(data).MalformedLines(); len(malformed) > 0 && !force {
		return fmt.Errorf("%s has unparseable line(s) %s — fix them, or push verbatim with --force", envCfg.Local, joinInts(malformed))
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

	m, remote, err := sess.Current(ctx, env)
	if err != nil {
		return err
	}
	if m != nil {
		base := p.st.Envs[env]
		if !force {
			if base.BaseBlobSHA256 == "" {
				return fmt.Errorf("exists on the server but was never pulled here — run `envbridge pull %s` first (or --force)", env)
			}
			if m.BlobSHA256 != base.BaseBlobSHA256 {
				return fmt.Errorf("changed on the server since your last pull (pushed by %s · %s) — pull, review, then push again (or --force)", m.UpdatedBy, ui.Ago(m.UpdatedAt))
			}
			drifted, _, err := sess.HandEditDrift(ctx, env, m)
			if err != nil {
				return err
			}
			if drifted {
				return fmt.Errorf("the server's %s was hand-edited — `envbridge pull %s --adopt` first, or --force to overwrite the hand-edit", m.MaterializePath, env)
			}
		}
		if store.SHA256Hex(data) == m.PlaintextSHA256 {
			if err := p.saveBase(env, m); err != nil {
				return err
			}
			fmt.Println(ui.Success(env + " already in sync"))
			return nil
		}
	}

	newM, err := sess.Write(ctx, env, data, envCfg.Materialize)
	if err != nil {
		return err
	}
	if err := p.saveBase(env, newM); err != nil {
		return err
	}

	changes := envdiff.Diff(envfile.Parse(remote), envfile.Parse(data))
	var added, removed, changed int
	for _, c := range changes {
		switch c.Kind {
		case envdiff.Added:
			added++
		case envdiff.Removed:
			removed++
		case envdiff.Changed:
			changed++
		}
	}
	fmt.Println(ui.Success(fmt.Sprintf("%s pushed (+%d ~%d -%d) · %s updated · backup kept", env, added, changed, removed, envCfg.Materialize)))
	return nil
}
