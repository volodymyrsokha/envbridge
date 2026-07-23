package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// NewLocal returns a Store on the local filesystem — for the binary running
// on the server itself, and for tests.
func NewLocal(root string) *FS { return &FS{fs: localFS{}, Root: root} }

type localFS struct{}

// expandLocalTilde mirrors the sftp adapter: config paths like ~/store must
// work identically when the same binary runs on the server via --local.
func expandLocalTilde(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}

func (localFS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(expandLocalTilde(path))
}

func (localFS) WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	return AtomicWriteFile(expandLocalTilde(path), data, mode)
}

func (localFS) CreateExcl(path string, data []byte) error {
	f, err := os.OpenFile(expandLocalTilde(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}

func (localFS) Remove(path string) error   { return os.Remove(expandLocalTilde(path)) }
func (localFS) MkdirAll(path string) error { return os.MkdirAll(expandLocalTilde(path), 0o755) }
func (localFS) Join(elem ...string) string { return filepath.Join(elem...) }
func (localFS) Dir(path string) string     { return filepath.Dir(path) }

func (localFS) ReadDirNames(path string) ([]string, error) {
	entries, err := os.ReadDir(expandLocalTilde(path))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// AtomicWriteFile writes via a temp file in the target's directory plus
// rename, so readers never observe a partial file.
func AtomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
