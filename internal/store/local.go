package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var ErrNotFound = errors.New("not found in store")

const (
	lockStaleAfter = 10 * time.Minute
	backupsKept    = 10
)

// Local implements Store directly on the filesystem: for the binary running
// on the server itself, and for tests.
type Local struct {
	Root string
}

func NewLocal(root string) *Local { return &Local{Root: root} }

func (l *Local) blobPath(env string) string     { return filepath.Join(l.Root, env+".env.age") }
func (l *Local) manifestPath(env string) string { return filepath.Join(l.Root, env+".manifest.json") }
func (l *Local) lockPath(env string) string     { return filepath.Join(l.Root, "locks", env+".lock") }
func (l *Local) backupDir(env string) string    { return filepath.Join(l.Root, "backups", env) }

func (l *Local) Init(ctx context.Context) error {
	for _, dir := range []string{l.Root, filepath.Join(l.Root, "locks"), filepath.Join(l.Root, "backups")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("cannot create store directory: %w — if this is a permission problem, run: sudo mkdir -p %s && sudo chown $USER %s", err, l.Root, l.Root)
		}
	}
	return nil
}

func (l *Local) ReadManifest(ctx context.Context, env string) (*Manifest, error) {
	data, err := os.ReadFile(l.manifestPath(env))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("environment %q: %w", env, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("corrupt manifest for %q: %w", env, err)
	}
	return &m, nil
}

func (l *Local) ReadBlob(ctx context.Context, env string) ([]byte, error) {
	data, err := os.ReadFile(l.blobPath(env))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("environment %q: %w", env, ErrNotFound)
	}
	return data, err
}

func (l *Local) WriteBlob(ctx context.Context, env string, blob []byte, m *Manifest) error {
	if err := l.backupCurrent(env); err != nil {
		return err
	}
	if err := atomicWrite(l.blobPath(env), blob, 0o644); err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(l.manifestPath(env), append(manifest, '\n'), 0o644)
}

func (l *Local) ReadMaterialized(ctx context.Context, env string) ([]byte, error) {
	m, err := l.ReadManifest(ctx, env)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(m.MaterializePath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("materialized file %s: %w", m.MaterializePath, ErrNotFound)
	}
	return data, err
}

func (l *Local) WriteMaterialized(ctx context.Context, env string, plaintext []byte) error {
	m, err := l.ReadManifest(ctx, env)
	if err != nil {
		return err
	}
	if err := atomicWrite(m.MaterializePath, plaintext, 0o600); err != nil {
		return fmt.Errorf("cannot write %s: %w", m.MaterializePath, err)
	}
	return nil
}

func (l *Local) Lock(ctx context.Context, env string, info LockInfo) (func() error, error) {
	path := l.lockPath(env)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			data, _ := json.Marshal(info)
			f.Write(data)
			f.Close()
			return func() error { return os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		holder := readLockInfo(path)
		if attempt == 0 && holder != nil && time.Since(holder.At) > lockStaleAfter {
			os.Remove(path)
			continue
		}
		if holder != nil {
			return nil, fmt.Errorf("%s is locked by %s (since %s) — retry when their operation finishes", env, holder.Who, holder.At.Format(time.RFC3339))
		}
		return nil, fmt.Errorf("%s is locked (unreadable lock file %s) — remove it manually if no operation is in flight", env, path)
	}
}

func readLockInfo(path string) *LockInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var info LockInfo
	if json.Unmarshal(data, &info) != nil {
		return nil
	}
	return &info
}

func (l *Local) backupCurrent(env string) error {
	current, err := os.ReadFile(l.blobPath(env))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := l.backupDir(env)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := time.Now().UTC().Format("2006-01-02T15-04-05.000000000Z") + ".env.age"
	if err := os.WriteFile(filepath.Join(dir, name), current, 0o644); err != nil {
		return err
	}
	return l.pruneBackups(dir)
}

func (l *Local) pruneBackups(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names[:max(0, len(names)-backupsKept)] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

// atomicWrite writes via a temp file in the target's directory plus rename,
// so readers never observe a partial file.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
