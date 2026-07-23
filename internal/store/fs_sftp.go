package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/pkg/sftp"
)

// NewSFTP returns a Store operating on a remote server over an established
// SFTP session. pkg/sftp maps SSH_FX_NO_SUCH_FILE to os.ErrNotExist, which
// the shared FS logic relies on.
func NewSFTP(client *sftp.Client, root string) *FS {
	return &FS{fs: &sftpFS{c: client}, Root: root}
}

type sftpFS struct {
	c        *sftp.Client
	home     string
	homeOnce sync.Once
}

// expand resolves a leading ~ against the remote home directory — SFTP has
// no shell, so tilde paths from the config would otherwise create a literal
// "~" directory on the server.
func (s *sftpFS) expand(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	s.homeOnce.Do(func() { s.home, _ = s.c.Getwd() })
	if s.home == "" {
		return p
	}
	return path.Join(s.home, strings.TrimPrefix(p, "~"))
}

func (s *sftpFS) ReadFile(p string) ([]byte, error) {
	f, err := s.c.Open(s.expand(p))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (s *sftpFS) WriteFileAtomic(p string, data []byte, mode os.FileMode) error {
	p = s.expand(p)
	tmp := p + ".tmp-" + randHex(6)
	f, err := s.c.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		s.c.Remove(tmp)
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		s.c.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		s.c.Remove(tmp)
		return err
	}
	// POSIX rename overwrites the target; fall back for servers without the
	// openssh extension.
	if err := s.c.PosixRename(tmp, p); err != nil {
		s.c.Remove(p)
		if err := s.c.Rename(tmp, p); err != nil {
			s.c.Remove(tmp)
			return err
		}
	}
	return nil
}

func (s *sftpFS) CreateExcl(p string, data []byte) error {
	f, err := s.c.OpenFile(s.expand(p), os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}

func (s *sftpFS) Remove(p string) error      { return s.c.Remove(s.expand(p)) }
func (s *sftpFS) MkdirAll(p string) error    { return s.c.MkdirAll(s.expand(p)) }
func (s *sftpFS) Join(elem ...string) string { return path.Join(elem...) }
func (s *sftpFS) Dir(p string) string        { return path.Dir(p) }

func (s *sftpFS) ReadDirNames(p string) ([]string, error) {
	infos, err := s.c.ReadDir(s.expand(p))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, info := range infos {
		if !info.IsDir() {
			names = append(names, info.Name())
		}
	}
	return names, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
