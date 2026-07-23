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

func newPullCmd() *cobra.Command {
	var adopt, force bool
	cmd := &cobra.Command{
		Use:   "pull [env...]",
		Short: "Fetch, decrypt, and write local env files",
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
				if err := pullOne(cmd.Context(), p, env, adopt, force); err != nil {
					return fmt.Errorf("%s: %w", env, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&adopt, "adopt", false, "adopt hand-edits found on the server as the new canonical version")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite local changes without confirmation")
	return cmd
}

func pullOne(ctx context.Context, p *project, env string, adopt, force bool) error {
	envCfg, err := p.envConfig(env)
	if err != nil {
		return err
	}
	sess, err := p.sessionFor(env)
	if err != nil {
		return err
	}
	m, remote, err := sess.Current(ctx, env)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("not in the store yet — push it first, or run `envbridge edit %s --local` on the server to import the existing file", env)
	}

	drifted, materialized, err := sess.HandEditDrift(ctx, env, m)
	if err != nil {
		return err
	}
	if drifted {
		if !adopt {
			return fmt.Errorf("the server's %s was hand-edited since the last envbridge write — `envbridge pull %s --adopt` makes the hand-edit canonical, `envbridge diff %s --server` shows what changed", m.MaterializePath, env, env)
		}
		unlock, err := sess.Store.Lock(ctx, env, store.LockInfo{Who: whoami(), Host: hostname(), At: time.Now().UTC()})
		if err != nil {
			return err
		}
		m, err = sess.Write(ctx, env, materialized, envCfg.Materialize)
		releaseLock(unlock)
		if err != nil {
			return err
		}
		remote = materialized
		fmt.Println(ui.Success("adopted the server's hand-edited content as canonical"))
	}

	localPath := p.localPath(envCfg)
	if current, err := os.ReadFile(localPath); err == nil {
		if string(current) == string(remote) {
			if err := p.saveBase(env, m); err != nil {
				return err
			}
			fmt.Println(ui.Success(env + " already up to date"))
			return nil
		}
		base := p.st.Envs[env]
		if store.SHA256Hex(current) != base.BasePlaintextSHA256 && !force {
			fmt.Printf("%s has local changes that pulling would overwrite:\n\n", envCfg.Local)
			fmt.Println(ui.RenderDiff(envdiff.Diff(envfile.Parse(remote), envfile.Parse(current)), false))
			fmt.Println()
			if !ui.Confirm("Discard these local changes?") {
				return fmt.Errorf("aborted — local file untouched (push your changes or rerun with --force)")
			}
		}
	}

	if err := store.AtomicWriteFile(localPath, remote, 0o600); err != nil {
		return err
	}
	if err := p.saveBase(env, m); err != nil {
		return err
	}
	fmt.Println(ui.Success(fmt.Sprintf("%s pulled → %s (last push %s · %s)", env, envCfg.Local, m.UpdatedBy, ui.Ago(m.UpdatedAt))))
	return nil
}
