package sshx

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyCallback verifies against known_hosts. Unknown hosts go through the
// TrustPrompt (refused when nil); a key mismatch is always a hard error —
// envbridge never silently skips host verification.
func (d *Dialer) hostKeyCallback() (ssh.HostKeyCallback, error) {
	path := d.KnownHostsPath
	if err := ensureFile(path); err != nil {
		return nil, fmt.Errorf("cannot prepare %s: %w", path, err)
	}
	verify, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := verify(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			return fmt.Errorf("HOST KEY MISMATCH for %s — the server's key changed, which can mean a machine-in-the-middle attack. If the server was legitimately reinstalled, remove its old line from %s and reconnect", hostname, path)
		}
		fingerprint := ssh.FingerprintSHA256(key)
		if d.TrustPrompt != nil && d.TrustPrompt(hostname, fingerprint) {
			return appendKnownHost(path, hostname, key)
		}
		return fmt.Errorf("unknown host %s (fingerprint %s) — not trusted", hostname, fingerprint)
	}, nil
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	_, werr := f.WriteString(line + "\n")
	return errors.Join(werr, f.Close())
}

func ensureFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}
