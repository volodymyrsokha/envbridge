// Package sshx dials SSH hosts the way users expect from the ssh binary:
// ~/.ssh/config aliases (HostName, User, Port, IdentityFile, ProxyJump),
// ssh-agent with identity-file fallback, and known_hosts verification with
// an explicit trust-on-first-use prompt. ProxyCommand, Match, and
// ControlMaster are not interpreted.
package sshx

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/kevinburke/ssh_config"
)

type Target struct {
	Alias         string
	HostName      string
	User          string
	Port          string
	IdentityFiles []string
	ProxyJump     string
}

// Resolve turns an environment's host string — an ssh_config alias, plain
// hostname, or explicit user@host:port — into a dialable target. Parts
// written explicitly in the string win over ssh_config values, which win
// over defaults.
func (d *Dialer) Resolve(alias string) Target {
	t := Target{Alias: alias}

	rest := alias
	if u, h, ok := strings.Cut(rest, "@"); ok && u != "" && h != "" {
		t.User = u
		rest = h
	}
	explicitPort := ""
	if h, p, ok := strings.Cut(rest, ":"); ok && p != "" {
		explicitPort = p
		rest = h
	}

	t.HostName = d.get(rest, "HostName")
	if t.HostName == "" {
		t.HostName = rest
	}
	if t.User == "" {
		t.User = d.get(rest, "User")
	}
	if t.User == "" {
		if u, err := user.Current(); err == nil {
			t.User = u.Username
		}
	}
	t.Port = explicitPort
	if t.Port == "" {
		t.Port = d.get(rest, "Port")
	}
	if t.Port == "" {
		t.Port = "22"
	}

	for _, f := range d.getAll(rest, "IdentityFile") {
		if f == ssh_config.Default("IdentityFile") {
			continue
		}
		t.IdentityFiles = append(t.IdentityFiles, expandHome(f))
	}
	if len(t.IdentityFiles) == 0 {
		for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			t.IdentityFiles = append(t.IdentityFiles, expandHome("~/.ssh/"+name))
		}
	}

	if j := d.get(rest, "ProxyJump"); j != "" && j != "none" {
		t.ProxyJump = j
	}
	return t
}

// get treats ssh_config's built-in defaults as "unset" so callers can apply
// their own precedence.
func (d *Dialer) get(alias, key string) string {
	v := d.GetOption(alias, key)
	if v == ssh_config.Default(key) {
		return ""
	}
	return v
}

func (d *Dialer) getAll(alias, key string) []string {
	if d.GetOptions != nil {
		return d.GetOptions(alias, key)
	}
	if v := d.GetOption(alias, key); v != "" {
		return []string{v}
	}
	return nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
