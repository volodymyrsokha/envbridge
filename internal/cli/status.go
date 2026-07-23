package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/config"
	"github.com/volodymyrsokha/envbridge/internal/store"
	"github.com/volodymyrsokha/envbridge/internal/ui"
)

type statusRow struct {
	Env      string `json:"env"`
	Local    string `json:"local"`
	Server   string `json:"server"`
	LastPush string `json:"last_push"`
	Error    string `json:"error,omitempty"`
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the sync state of every environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := openProject()
			if err != nil {
				return err
			}
			defer p.Close()

			var rows []statusRow
			for _, env := range p.cfg.EnvNames() {
				rows = append(rows, statusOne(cmd.Context(), p, env))
			}

			if flagJSON {
				out, err := json.MarshalIndent(rows, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(out))
				return nil
			}
			renderStatusTable(rows)
			return nil
		},
	}
}

func statusOne(ctx context.Context, p *project, env string) statusRow {
	row := statusRow{Env: env}
	envCfg, err := p.envConfig(env)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	sess, err := p.sessionFor(env)
	if err != nil {
		row.Error = err.Error()
		return row
	}

	m, err := sess.Store.ReadManifest(ctx, env)
	if errors.Is(err, store.ErrNotFound) {
		row.Server = "– not set up"
		row.LastPush = "never"
		row.Local = localColumn(p, env, envCfg, nil)
		return row
	}
	if err != nil {
		row.Error = err.Error()
		return row
	}

	row.Server = "✓ clean"
	if drifted, _, derr := sess.HandEditDrift(ctx, env, m); derr == nil && drifted {
		row.Server = "⚠ hand-edited"
	}
	if m.RecipientsFingerprint != store.RecipientsFingerprint(p.cfg.RecipientKeys()) {
		row.Server += ", ⚠ recipients outdated"
	}
	row.LastPush = m.UpdatedBy + " · " + ui.Ago(m.UpdatedAt)
	row.Local = localColumn(p, env, envCfg, m)
	return row
}

func localColumn(p *project, env string, envCfg config.Environment, m *store.Manifest) string {
	current, err := os.ReadFile(p.localPath(envCfg))
	if err != nil {
		return "– not pulled"
	}
	base := p.st.Envs[env]
	modified := store.SHA256Hex(current) != base.BasePlaintextSHA256
	behind := m != nil && base.BaseBlobSHA256 != "" && m.BlobSHA256 != base.BaseBlobSHA256
	switch {
	case modified && behind:
		return "⚠ modified + behind"
	case modified:
		return "● modified"
	case behind:
		return "↓ behind"
	default:
		return "✓ in sync"
	}
}

func renderStatusTable(rows []statusRow) {
	envW, localW, serverW := len("ENV"), len("LOCAL"), len("SERVER")
	for _, r := range rows {
		envW = max(envW, len(r.Env))
		localW = max(localW, len([]rune(r.Local)))
		serverW = max(serverW, len([]rune(r.Server)))
	}
	fmt.Printf("  %-*s  %-*s  %-*s  %s\n", envW, "ENV", localW, "LOCAL", serverW, "SERVER", "LAST PUSH")
	for _, r := range rows {
		if r.Error != "" {
			fmt.Printf("  %-*s  %s\n", envW, r.Env, ui.RenderError(errors.New(r.Error)))
			continue
		}
		fmt.Printf("  %-*s  %-*s  %-*s  %s\n", envW, r.Env, localW+padAdjust(r.Local), r.Local, serverW+padAdjust(r.Server), r.Server, r.LastPush)
	}
}

// padAdjust compensates %-*s byte-width padding for multi-byte glyphs like ✓.
func padAdjust(s string) int {
	return len(s) - len([]rune(s))
}
