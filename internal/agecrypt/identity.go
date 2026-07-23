package agecrypt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
)

func errorsAs(err error, target any) bool { return errors.As(err, target) }

// GenerateIdentity creates a new X25519 identity and writes it to path in
// the standard age identity file format, refusing to overwrite an existing
// file. The parent directory is created 0700, the file 0600.
func GenerateIdentity(path string) (*age.X25519Identity, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("identity already exists at %s — delete it first if you really want a new one (teammates would need to re-add you)", path)
		}
		return nil, err
	}
	content := fmt.Sprintf("# created: %s\n# public key: %s\n%s\n",
		time.Now().Format(time.RFC3339), id.Recipient(), id)
	_, werr := f.WriteString(content)
	if err := errors.Join(werr, f.Close()); err != nil {
		return nil, err
	}
	return id, nil
}

// LoadIdentity reads an age identity file, rejecting files readable by group
// or others.
func LoadIdentity(path string) (*age.X25519Identity, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no identity found at %s — run `envbridge keygen` to create one", path)
		}
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("identity file %s is readable by others (mode %04o) — run: chmod 600 %s", path, info.Mode().Perm(), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "AGE-SECRET-KEY-1") {
			return age.ParseX25519Identity(line)
		}
	}
	return nil, fmt.Errorf("no AGE-SECRET-KEY found in %s", path)
}
