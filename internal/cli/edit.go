package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/agecrypt"
	"github.com/volodymyrsokha/envbridge/internal/config"
	"github.com/volodymyrsokha/envbridge/internal/envdiff"
	"github.com/volodymyrsokha/envbridge/internal/envfile"
	"github.com/volodymyrsokha/envbridge/internal/state"
	"github.com/volodymyrsokha/envbridge/internal/store"
	"github.com/volodymyrsokha/envbridge/internal/ui"
	"github.com/volodymyrsokha/envbridge/internal/version"
)

func newEditCmd() *cobra.Command {
	var local, adopt, yes bool
	cmd := &cobra.Command{
		Use:   "edit <env>",
		Short: "Decrypt, edit in $EDITOR, re-encrypt, and push",
		Long: `Opens the environment in your editor via a locked decrypt/re-encrypt
round-trip. Works on a dev machine over SSH and directly on the server —
this replaces hand-editing the live file.

If the environment doesn't exist in the store yet but the materialized file
does, its current content is imported as the starting point.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !local {
				return fmt.Errorf("remote editing lands in M4 — on the server, run with --local")
			}
			return runEditLocal(cmd.Context(), args[0], adopt, yes)
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "operate on the local filesystem store (when running on the server)")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "start from the hand-edited materialized file instead of aborting on drift")
	cmd.Flags().BoolVar(&yes, "yes", false, "push without the confirmation prompt")
	return cmd
}

func runEditLocal(ctx context.Context, env string, adopt, yes bool) error {
	cfg, projectRoot, err := config.Discover(".")
	if err != nil {
		return err
	}
	envCfg, ok := cfg.Environments[env]
	if !ok {
		return fmt.Errorf("unknown environment %q — available: %s", env, strings.Join(cfg.EnvNames(), ", "))
	}

	idPath, err := config.IdentityPath()
	if err != nil {
		return err
	}
	identity, err := agecrypt.LoadIdentity(idPath)
	if err != nil {
		return err
	}

	st := store.NewLocal(cfg.StoreFor(env))
	if err := st.Init(ctx); err != nil {
		return err
	}
	sess := &store.Session{
		Store:       st,
		Identity:    identity,
		Recipients:  cfg.RecipientKeys(),
		UpdatedBy:   whoami(),
		ToolVersion: version.Version,
	}

	unlock, err := st.Lock(ctx, env, store.LockInfo{Who: sess.UpdatedBy, Host: hostname(), At: time.Now().UTC()})
	if err != nil {
		return err
	}
	defer unlock()

	manifest, current, err := sess.Current(ctx, env)
	if err != nil {
		return err
	}

	start := current
	if manifest == nil {
		if content, err := os.ReadFile(envCfg.Materialize); err == nil {
			start = content
			fmt.Println(ui.Success(fmt.Sprintf("importing existing %s as the initial content", envCfg.Materialize)))
		}
	} else {
		drifted, materialized, err := sess.HandEditDrift(ctx, env, manifest)
		if err != nil {
			return err
		}
		if drifted {
			if !adopt {
				return fmt.Errorf("%s was hand-edited on the server since the last envbridge write — rerun with --adopt to start from the hand-edited content, or resolve the drift first", envCfg.Materialize)
			}
			start = materialized
			fmt.Println(ui.Success("adopting hand-edited content as the starting point"))
		}
	}

	edited, err := editorRoundTrip(env, start)
	if err != nil {
		return err
	}

	if manifest != nil && bytes.Equal(edited, current) {
		fmt.Println("no changes — nothing pushed")
		return nil
	}

	changes := envdiff.Diff(envfile.Parse(current), envfile.Parse(edited))
	fmt.Println()
	fmt.Println(ui.RenderDiff(changes, false))
	fmt.Println()
	if !yes && !ui.Confirm(fmt.Sprintf("Push %s?", env)) {
		return fmt.Errorf("aborted — nothing pushed")
	}

	m, err := sess.Write(ctx, env, edited, envCfg.Materialize)
	if err != nil {
		return err
	}

	s, err := state.Load(projectRoot)
	if err == nil {
		s.Envs[env] = state.EnvState{
			BaseBlobSHA256:      m.BlobSHA256,
			BasePlaintextSHA256: m.PlaintextSHA256,
			PulledAt:            time.Now().UTC(),
		}
		err = s.Save(projectRoot)
	}
	if err != nil {
		fmt.Println(ui.Hint("could not update local state: " + err.Error()))
	}

	fmt.Println(ui.Success(fmt.Sprintf("%s pushed · %s updated · backup kept", env, envCfg.Materialize)))
	return nil
}

// editorRoundTrip decrypts into a private tempfile, runs the editor, and
// re-parses — looping back into the editor on syntax errors so typed work is
// never lost. The tempfile is zeroed before removal on every path.
func editorRoundTrip(env string, content []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "envbridge-*")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, env+".env")
	defer func() {
		if info, err := os.Stat(path); err == nil {
			os.WriteFile(path, make([]byte, info.Size()), 0o600)
		}
		os.RemoveAll(dir)
	}()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return nil, err
	}

	for {
		if err := runEditor(path); err != nil {
			return nil, err
		}
		edited, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		malformed := envfile.Parse(edited).MalformedLines()
		if len(malformed) == 0 {
			return edited, nil
		}
		fmt.Println(ui.RenderError(fmt.Errorf("cannot parse line(s) %s", joinInts(malformed))))
		if !ui.Confirm("Reopen the editor to fix it?") {
			return nil, fmt.Errorf("aborted — the file has syntax errors on line(s) %s", joinInts(malformed))
		}
	}
}

func runEditor(path string) error {
	parts := strings.Fields(config.Editor())
	parts = append(parts, path)
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q failed: %w", parts[0], err)
	}
	return nil
}

func whoami() string {
	if u, err := user.Current(); err == nil {
		return u.Username + "@" + hostname()
	}
	return "unknown@" + hostname()
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ", ")
}
