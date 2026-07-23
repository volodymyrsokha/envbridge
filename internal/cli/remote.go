package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/volodymyrsokha/envbridge/internal/agecrypt"
	"github.com/volodymyrsokha/envbridge/internal/config"
	"github.com/volodymyrsokha/envbridge/internal/sshx"
	"github.com/volodymyrsokha/envbridge/internal/state"
	"github.com/volodymyrsokha/envbridge/internal/store"
	"github.com/volodymyrsokha/envbridge/internal/ui"
	"github.com/volodymyrsokha/envbridge/internal/version"
)

// project holds everything a command needs: config, identity, state, and
// lazily-dialed SSH/SFTP connections cached per host.
type project struct {
	cfg      *config.Project
	root     string
	identity *age.X25519Identity
	st       *state.State
	dialer   *sshx.Dialer
	ssh      map[string]*ssh.Client
	sftp     map[string]*sftp.Client
}

func openProject() (*project, error) {
	cfg, root, err := config.Discover(".")
	if err != nil {
		return nil, err
	}
	idPath, err := config.IdentityPath()
	if err != nil {
		return nil, err
	}
	identity, err := agecrypt.LoadIdentity(idPath)
	if err != nil {
		return nil, err
	}
	st, err := state.Load(root)
	if err != nil {
		return nil, err
	}
	dialer := sshx.DefaultDialer()
	dialer.TrustPrompt = func(host, fingerprint string) bool {
		fmt.Fprintf(os.Stderr, "The authenticity of host %s can't be established.\nKey fingerprint: %s\n", host, fingerprint)
		return ui.Confirm("Trust this host and add it to known_hosts?")
	}
	return &project{
		cfg:      cfg,
		root:     root,
		identity: identity,
		st:       st,
		dialer:   dialer,
		ssh:      map[string]*ssh.Client{},
		sftp:     map[string]*sftp.Client{},
	}, nil
}

func (p *project) Close() {
	for _, c := range p.sftp {
		_ = c.Close()
	}
	for _, c := range p.ssh {
		_ = c.Close()
	}
}

func (p *project) envConfig(env string) (config.Environment, error) {
	e, ok := p.cfg.Environments[env]
	if !ok {
		return config.Environment{}, fmt.Errorf("unknown environment %q — available: %s", env, strings.Join(p.cfg.EnvNames(), ", "))
	}
	return e, nil
}

// envList expands no args to every configured environment.
func (p *project) envList(args []string) ([]string, error) {
	if len(args) == 0 {
		return p.cfg.EnvNames(), nil
	}
	for _, env := range args {
		if _, err := p.envConfig(env); err != nil {
			return nil, err
		}
	}
	return args, nil
}

func (p *project) storeFor(env string) (store.Store, error) {
	envCfg, err := p.envConfig(env)
	if err != nil {
		return nil, err
	}
	host := envCfg.Host
	if c, ok := p.sftp[host]; ok {
		return store.NewSFTP(c, p.cfg.StoreFor(env)), nil
	}
	fmt.Fprintf(os.Stderr, "· connecting to %s…\n", host)
	sshClient, err := p.dialer.Dial(host)
	if err != nil {
		return nil, err
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("%s: SFTP subsystem unavailable: %w", host, err)
	}
	p.ssh[host] = sshClient
	p.sftp[host] = sftpClient
	return store.NewSFTP(sftpClient, p.cfg.StoreFor(env)), nil
}

func (p *project) sessionFor(env string) (*store.Session, error) {
	st, err := p.storeFor(env)
	if err != nil {
		return nil, err
	}
	return p.sessionOn(st), nil
}

func (p *project) sessionOn(st store.Store) *store.Session {
	return &store.Session{
		Store:       st,
		Identity:    p.identity,
		Recipients:  p.cfg.RecipientKeys(),
		UpdatedBy:   whoami(),
		ToolVersion: version.Version,
	}
}

func (p *project) localPath(envCfg config.Environment) string {
	if filepath.IsAbs(envCfg.Local) {
		return envCfg.Local
	}
	return filepath.Join(p.root, envCfg.Local)
}

func (p *project) saveBase(env string, m *store.Manifest) error {
	p.st.Envs[env] = state.EnvState{
		BaseBlobSHA256:      m.BlobSHA256,
		BasePlaintextSHA256: m.PlaintextSHA256,
		PulledAt:            m.UpdatedAt,
	}
	return p.st.Save(p.root)
}
