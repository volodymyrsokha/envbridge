package sshx

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const maxJumpDepth = 8

type Dialer struct {
	// GetOption/GetOptions look up ssh_config values; tests inject their own.
	GetOption      func(alias, key string) string
	GetOptions     func(alias, key string) []string
	KnownHostsPath string
	// TrustPrompt decides whether to accept an unknown host key. nil refuses.
	TrustPrompt func(host, fingerprint string) bool
	Timeout     time.Duration
}

// DefaultDialer loads ~/.ssh/config itself (resolved via os.UserHomeDir, so
// $HOME overrides work) — ssh_config's package-level Get resolves the home
// directory through the passwd database and ignores $HOME.
func DefaultDialer() *Dialer {
	home, _ := os.UserHomeDir()
	cfg := loadSSHConfig(filepath.Join(home, ".ssh", "config"))
	return &Dialer{
		GetOption: func(alias, key string) string {
			if cfg != nil {
				if v, err := cfg.Get(alias, key); err == nil && v != "" {
					return v
				}
			}
			return ssh_config.Default(key)
		},
		GetOptions: func(alias, key string) []string {
			if cfg != nil {
				if vs, err := cfg.GetAll(alias, key); err == nil && len(vs) > 0 {
					return vs
				}
			}
			return nil
		},
		KnownHostsPath: filepath.Join(home, ".ssh", "known_hosts"),
		Timeout:        15 * time.Second,
	}
}

func loadSSHConfig(path string) *ssh_config.Config {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil
	}
	return cfg
}

func (d *Dialer) Dial(alias string) (*ssh.Client, error) {
	return d.dialVia(nil, alias, 0)
}

func (d *Dialer) dialVia(via *ssh.Client, alias string, depth int) (*ssh.Client, error) {
	if depth > maxJumpDepth {
		return nil, fmt.Errorf("ProxyJump chain deeper than %d hops — refusing", maxJumpDepth)
	}
	t := d.Resolve(alias)

	if t.ProxyJump != "" {
		for _, jump := range strings.Split(t.ProxyJump, ",") {
			j, err := d.dialVia(via, strings.TrimSpace(jump), depth+1)
			if err != nil {
				return nil, fmt.Errorf("via jump host %s: %w", jump, err)
			}
			via = j
		}
	}

	hostKeyCB, err := d.hostKeyCallback()
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            t.User,
		Auth:            d.authMethods(t),
		HostKeyCallback: hostKeyCB,
		Timeout:         d.Timeout,
	}
	addr := net.JoinHostPort(t.HostName, t.Port)

	var client *ssh.Client
	if via == nil {
		client, err = ssh.Dial("tcp", addr, cfg)
	} else {
		conn, cerr := via.Dial("tcp", addr)
		if cerr != nil {
			return nil, fmt.Errorf("cannot reach %s through jump host: %w", addr, cerr)
		}
		ncc, chans, reqs, nerr := ssh.NewClientConn(conn, addr, cfg)
		if nerr != nil {
			err = nerr
		} else {
			client = ssh.NewClient(ncc, chans, reqs)
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, fmt.Errorf("cannot authenticate to %s as %s — is your key loaded in ssh-agent (`ssh-add`) or configured via IdentityFile in ~/.ssh/config?", addr, t.User)
		}
		return nil, fmt.Errorf("cannot connect to %s (%s): %w", t.Alias, addr, err)
	}
	return client, nil
}

func (d *Dialer) authMethods(t Target) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	files := t.IdentityFiles
	methods = append(methods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		return loadSigners(files), nil
	}))
	return methods
}

// loadSigners parses whatever identity files exist and are unencrypted;
// passphrase-protected keys are skipped (the agent handles those).
func loadSigners(files []string) []ssh.Signer {
	var signers []ssh.Signer
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			continue
		}
		signers = append(signers, signer)
	}
	return signers
}
