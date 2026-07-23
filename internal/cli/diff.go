package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/envdiff"
	"github.com/volodymyrsokha/envbridge/internal/envfile"
	"github.com/volodymyrsokha/envbridge/internal/ui"
)

func newDiffCmd() *cobra.Command {
	var showValues, server bool
	cmd := &cobra.Command{
		Use:   "diff [env]",
		Short: "Show key-level differences between local and server",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := openProject()
			if err != nil {
				return err
			}
			defer p.Close()

			var env string
			switch {
			case len(args) == 1:
				env = args[0]
			case len(p.cfg.Environments) == 1:
				env = p.cfg.EnvNames()[0]
			default:
				return fmt.Errorf("which environment? available: %s", joinStrings(p.cfg.EnvNames()))
			}
			envCfg, err := p.envConfig(env)
			if err != nil {
				return err
			}
			sess, err := p.sessionFor(env)
			if err != nil {
				return err
			}
			m, remote, err := sess.Current(cmd.Context(), env)
			if err != nil {
				return err
			}
			if m == nil {
				return fmt.Errorf("%s is not in the store yet — nothing to diff against", env)
			}

			var from, to []byte
			var header string
			if server {
				from = remote
				to, err = sess.Store.ReadMaterialized(cmd.Context(), env)
				if err != nil {
					return err
				}
				header = fmt.Sprintf("%s — canonical → materialized (hand-edits on the server)", env)
			} else {
				from = remote
				to, err = os.ReadFile(p.localPath(envCfg))
				if os.IsNotExist(err) {
					fmt.Println(ui.Hint(envCfg.Local + " does not exist locally — showing what `pull` would create"))
					err = nil
				}
				if err != nil {
					return err
				}
				header = fmt.Sprintf("%s — server → local (what `push` would change)", env)
			}

			changes := envdiff.Diff(envfile.Parse(from), envfile.Parse(to))
			if len(changes) == 0 {
				fmt.Println(ui.Success(env + ": no differences"))
				return nil
			}
			fmt.Println(header)
			fmt.Println()
			fmt.Println(ui.RenderDiff(changes, showValues))
			if !showValues {
				fmt.Println()
				fmt.Println(ui.Hint("values are masked — use --show-values to reveal them"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&showValues, "show-values", false, "reveal secret values instead of masking them")
	cmd.Flags().BoolVar(&server, "server", false, "diff the canonical blob against the server's materialized file")
	return cmd
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
