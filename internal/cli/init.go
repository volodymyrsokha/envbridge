package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/spf13/cobra"

	"github.com/volodymyrsokha/envbridge/internal/agecrypt"
	"github.com/volodymyrsokha/envbridge/internal/config"
	"github.com/volodymyrsokha/envbridge/internal/envfile"
	"github.com/volodymyrsokha/envbridge/internal/store"
	"github.com/volodymyrsokha/envbridge/internal/ui"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a new project or join an existing one",
		Long: `With no .envbridge.yaml in the repository, runs the project wizard:
environments, hosts, store setup on each server, and import of existing
env files as the initial encrypted blobs.

With an existing .envbridge.yaml, sets up your identity and prints the
public key a teammate adds with ` + "`envbridge team add`" + `.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := config.Discover("."); err == nil {
				return runInitJoin(cmd.Context())
			}
			return runInitBootstrap(cmd.Context())
		},
	}
}

func runInitJoin(ctx context.Context) error {
	cfg, root, err := config.Discover(".")
	if err != nil {
		return err
	}
	identity, err := ensureIdentity()
	if err != nil {
		return err
	}
	if err := ensureGitignore(root, cfg); err != nil {
		return err
	}

	pub := identity.Recipient().String()
	member := false
	for _, key := range cfg.RecipientKeys() {
		if key == pub {
			member = true
			break
		}
	}
	if member {
		fmt.Println(ui.Success("you are already a recipient — checking connectivity"))
		checkConnectivity(ctx, cfg)
		fmt.Println()
		fmt.Println("Next: `envbridge pull` to fetch your env files")
		return nil
	}

	fmt.Println("You're not in the recipients list yet. Your public key:")
	fmt.Println()
	fmt.Println("  " + ui.Emphasize(pub))
	fmt.Println()
	fmt.Println("Ask a teammate to run:")
	fmt.Printf("  envbridge team add --name %q --email %q --key %s\n",
		gitConfig("user.name", "Your Name"), gitConfig("user.email", "you@example.com"), pub)
	fmt.Println()
	fmt.Println("After they push the config change and run `envbridge team sync`,")
	fmt.Println("run `envbridge pull`.")
	return nil
}

func runInitBootstrap(ctx context.Context) error {
	fmt.Fprintln(os.Stderr, "No "+config.ProjectFileName+" found — setting up a new project.")
	fmt.Fprintln(os.Stderr, "")

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	name := ui.Ask("Project name?", filepath.Base(cwd))
	storeRoot := ui.Ask("Store directory on the server(s)?", "/var/lib/envbridge/"+name)

	envs := map[string]config.Environment{}
	for {
		prompt := "Environment name?"
		def := ""
		if len(envs) == 0 {
			def = "production"
		} else {
			prompt = "Another environment? (enter to finish)"
		}
		envName := ui.Ask(prompt, def)
		if envName == "" {
			break
		}
		host := ui.AskRequired("  SSH host for " + envName + " (~/.ssh/config alias or user@host)?")
		materialize := ui.AskRequired("  Path the app reads its .env from on that server?")
		local := ui.Ask("  Local filename for pulled copies?", ".env."+envName)
		envs[envName] = config.Environment{Host: host, Materialize: materialize, Local: local}
	}
	if len(envs) == 0 {
		return fmt.Errorf("no environments configured — nothing to do")
	}

	identity, err := ensureIdentity()
	if err != nil {
		return err
	}
	selfName := ui.Ask("Your name (for the recipients list)?", gitConfig("user.name", ""))
	selfEmail := ui.Ask("Your email?", gitConfig("user.email", ""))

	cfg := &config.Project{
		Version:      1,
		Project:      name,
		Store:        storeRoot,
		Environments: envs,
		Recipients:   []config.Recipient{{Name: selfName, Email: selfEmail, Key: identity.Recipient().String()}},
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	path := filepath.Join(cwd, config.ProjectFileName)
	if err := os.WriteFile(path, []byte(renderProjectYAML(cfg)), 0o644); err != nil {
		return err
	}
	fmt.Println(ui.Success(config.ProjectFileName + " written — commit it so your team can join"))
	if err := ensureGitignore(cwd, cfg); err != nil {
		return err
	}

	p, err := openProject()
	if err != nil {
		return err
	}
	defer p.Close()
	for _, env := range p.cfg.EnvNames() {
		if err := initEnv(ctx, p, env); err != nil {
			fmt.Println(ui.RenderError(fmt.Errorf("%s: %v", env, err)))
			fmt.Println(ui.Hint("fix the problem and rerun `envbridge init` — it is safe to repeat"))
		}
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  • commit " + config.ProjectFileName + " and .gitignore")
	fmt.Println("  • teammates run `envbridge init` in the repo to join")
	return nil
}

// initEnv creates the store on the environment's server and imports the
// existing materialized file as the first encrypted version, if one exists.
func initEnv(ctx context.Context, p *project, env string) error {
	envCfg, err := p.envConfig(env)
	if err != nil {
		return err
	}
	st, err := p.storeFor(env)
	if err != nil {
		return err
	}
	if err := st.Init(ctx); err != nil {
		return err
	}
	sess := p.sessionOn(st)

	if m, err := st.ReadManifest(ctx, env); err == nil && m != nil {
		fmt.Println(ui.Success(env + " already in the store — leaving it untouched"))
		return nil
	}

	fs, ok := st.(*store.FS)
	if !ok {
		return nil
	}
	existing, err := fs.ReadPath(ctx, envCfg.Materialize)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println(ui.Success(env + " store ready (no existing " + envCfg.Materialize + " to import — first push creates it)"))
		return nil
	}
	if err != nil {
		return err
	}

	unlock, err := st.Lock(ctx, env, store.LockInfo{Who: whoami(), Host: hostname(), At: time.Now().UTC()})
	if err != nil {
		return err
	}
	defer unlock()
	m, err := sess.Write(ctx, env, existing, envCfg.Materialize)
	if err != nil {
		return err
	}
	if err := p.saveBase(env, m); err != nil {
		return err
	}
	keys := len(envfile.Parse(existing).Keys())
	fmt.Println(ui.Success(fmt.Sprintf("%s: imported existing %s (%d keys, encrypted)", env, envCfg.Materialize, keys)))
	return nil
}

func ensureIdentity() (*age.X25519Identity, error) {
	path, err := config.IdentityPath()
	if err != nil {
		return nil, err
	}
	identity, err := agecrypt.LoadIdentity(path)
	if err == nil {
		return identity, nil
	}
	identity, err = agecrypt.GenerateIdentity(path)
	if err != nil {
		return nil, err
	}
	fmt.Println(ui.Success("generated your age identity: " + path))
	return identity, nil
}

// ensureGitignore appends the pulled env files and local state dir, keeping
// whatever is already there.
func ensureGitignore(root string, cfg *config.Project) error {
	wanted := []string{".envbridge/"}
	for _, env := range cfg.EnvNames() {
		local := cfg.Environments[env].Local
		if !filepath.IsAbs(local) {
			wanted = append(wanted, local)
		}
	}

	path := filepath.Join(root, ".gitignore")
	existing := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		existing[strings.TrimSpace(line)] = true
	}

	var missing []string
	for _, entry := range wanted {
		if !existing[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n# envbridge — local env files stay out of git\n" + strings.Join(missing, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Println(ui.Success(".gitignore updated (" + strings.Join(missing, ", ") + ")"))
	return nil
}

func checkConnectivity(ctx context.Context, cfg *config.Project) {
	p, err := openProject()
	if err != nil {
		fmt.Println(ui.RenderError(err))
		return
	}
	defer p.Close()
	for _, env := range cfg.EnvNames() {
		st, err := p.storeFor(env)
		if err != nil {
			fmt.Println(ui.RenderError(fmt.Errorf("%s: %v", env, err)))
			continue
		}
		if _, err := st.ReadManifest(ctx, env); err != nil && !errors.Is(err, store.ErrNotFound) {
			fmt.Println(ui.RenderError(fmt.Errorf("%s: %v", env, err)))
			continue
		}
		fmt.Println(ui.Success(env + ": " + cfg.Environments[env].Host + " reachable"))
	}
}

func gitConfig(key, fallback string) string {
	out, err := exec.Command("git", "config", "--get", key).Output()
	if err != nil {
		return fallback
	}
	if v := strings.TrimSpace(string(out)); v != "" {
		return v
	}
	return fallback
}

func renderProjectYAML(cfg *config.Project) string {
	var b strings.Builder
	b.WriteString("# envbridge project configuration — commit this file.\n")
	b.WriteString("# Secrets never live here: recipients are PUBLIC keys.\n")
	b.WriteString("version: 1\n")
	b.WriteString("project: " + cfg.Project + "\n")
	b.WriteString("store: " + cfg.Store + "\n")
	b.WriteString("\nenvironments:\n")

	envNames := make([]string, 0, len(cfg.Environments))
	for name := range cfg.Environments {
		envNames = append(envNames, name)
	}
	sort.Strings(envNames)
	for _, name := range envNames {
		e := cfg.Environments[name]
		b.WriteString("  " + name + ":\n")
		b.WriteString("    host: " + e.Host + "          # ssh alias from ~/.ssh/config, or user@host:port\n")
		b.WriteString("    materialize: " + e.Materialize + "   # plaintext path the app reads on the server\n")
		b.WriteString("    local: " + e.Local + "       # written by `envbridge pull` (gitignored)\n")
	}

	b.WriteString("\n# Team members' age public keys. Add teammates with `envbridge team add`.\n")
	b.WriteString("recipients:\n")
	for _, r := range cfg.Recipients {
		b.WriteString("  - name: " + r.Name + "\n")
		b.WriteString("    email: " + r.Email + "\n")
		b.WriteString("    key: " + r.Key + "\n")
	}
	return b.String()
}
