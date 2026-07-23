package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

var ErrNotFound = errors.New("not found in store")

const (
	lockStaleAfter = 10 * time.Minute
	backupsKept    = 10
)

// fsys is the minimal filesystem surface the store logic needs. Adapters
// exist for the local filesystem and SFTP; both must map missing files to
// os.ErrNotExist.
type fsys interface {
	ReadFile(path string) ([]byte, error)
	WriteFileAtomic(path string, data []byte, mode os.FileMode) error
	CreateExcl(path string, data []byte) error
	Remove(path string) error
	MkdirAll(path string) error
	ReadDirNames(path string) ([]string, error)
	Join(elem ...string) string
	Dir(path string) string
}

// FS implements Store with identical semantics wherever the files live: on
// this machine (NewLocal) or on the team's server over SFTP (NewSFTP).
type FS struct {
	fs   fsys
	Root string
}

func (l *FS) blobPath(env string) string     { return l.fs.Join(l.Root, env+".env.age") }
func (l *FS) manifestPath(env string) string { return l.fs.Join(l.Root, env+".manifest.json") }
func (l *FS) lockPath(env string) string     { return l.fs.Join(l.Root, "locks", env+".lock") }
func (l *FS) backupDir(env string) string    { return l.fs.Join(l.Root, "backups", env) }

func (l *FS) Init(ctx context.Context) error {
	for _, dir := range []string{l.Root, l.fs.Join(l.Root, "locks"), l.fs.Join(l.Root, "backups")} {
		if err := l.fs.MkdirAll(dir); err != nil {
			return fmt.Errorf("cannot create store directory: %w — if this is a permission problem, run: sudo mkdir -p %s && sudo chown $USER %s", err, l.Root, l.Root)
		}
	}
	return nil
}

// ReadPath reads an arbitrary file wherever this store lives — init uses it
// to import a server's existing env file before any manifest exists.
func (l *FS) ReadPath(ctx context.Context, path string) ([]byte, error) {
	return l.fs.ReadFile(path)
}

func (l *FS) ReadManifest(ctx context.Context, env string) (*Manifest, error) {
	data, err := l.fs.ReadFile(l.manifestPath(env))
	if errors.Is(err, os.ErrNotExist) {
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

func (l *FS) ReadBlob(ctx context.Context, env string) ([]byte, error) {
	data, err := l.fs.ReadFile(l.blobPath(env))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("environment %q: %w", env, ErrNotFound)
	}
	return data, err
}

func (l *FS) WriteBlob(ctx context.Context, env string, blob []byte, m *Manifest) error {
	if err := l.backupCurrent(env); err != nil {
		return err
	}
	if err := l.fs.WriteFileAtomic(l.blobPath(env), blob, 0o644); err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return l.fs.WriteFileAtomic(l.manifestPath(env), append(manifest, '\n'), 0o644)
}

func (l *FS) ReadMaterialized(ctx context.Context, env string) ([]byte, error) {
	m, err := l.ReadManifest(ctx, env)
	if err != nil {
		return nil, err
	}
	data, err := l.fs.ReadFile(m.MaterializePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("materialized file %s: %w", m.MaterializePath, ErrNotFound)
	}
	return data, err
}

func (l *FS) WriteMaterialized(ctx context.Context, env string, plaintext []byte) error {
	m, err := l.ReadManifest(ctx, env)
	if err != nil {
		return err
	}
	if err := l.fs.MkdirAll(l.fs.Dir(m.MaterializePath)); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", m.MaterializePath, err)
	}
	if err := l.fs.WriteFileAtomic(m.MaterializePath, plaintext, 0o600); err != nil {
		return fmt.Errorf("cannot write %s: %w", m.MaterializePath, err)
	}
	return nil
}

func (l *FS) Lock(ctx context.Context, env string, info LockInfo) (func() error, error) {
	path := l.lockPath(env)
	// Creating the locks dir here lets the first push/edit against a fresh
	// server bootstrap the whole store, since every write starts with Lock.
	if err := l.fs.MkdirAll(l.fs.Join(l.Root, "locks")); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w — if this is a permission problem, run: sudo mkdir -p %s && sudo chown $USER %s", l.Root, err, l.Root, l.Root)
	}
	data, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		createErr := l.fs.CreateExcl(path, data)
		if createErr == nil {
			return func() error { return l.fs.Remove(path) }, nil
		}
		// O_EXCL failures aren't distinguishable across transports (SFTPv3
		// has no EEXIST), so inspect the lock file to find out what happened.
		existing, readErr := l.fs.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, createErr
		}
		var holder LockInfo
		if json.Unmarshal(existing, &holder) != nil || holder.Who == "" {
			return nil, fmt.Errorf("%s is locked (unreadable lock file %s) — remove it manually if no operation is in flight", env, path)
		}
		if time.Since(holder.At) > lockStaleAfter {
			_ = l.fs.Remove(path)
			continue
		}
		return nil, fmt.Errorf("%s is locked by %s (since %s) — retry when their operation finishes", env, holder.Who, holder.At.Format(time.RFC3339))
	}
	return nil, fmt.Errorf("could not acquire lock for %s", env)
}

func (l *FS) backupCurrent(env string) error {
	current, err := l.fs.ReadFile(l.blobPath(env))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := l.backupDir(env)
	if err := l.fs.MkdirAll(dir); err != nil {
		return err
	}
	name := time.Now().UTC().Format("2006-01-02T15-04-05.000000000Z") + ".env.age"
	if err := l.fs.WriteFileAtomic(l.fs.Join(dir, name), current, 0o644); err != nil {
		return err
	}
	return l.pruneBackups(dir)
}

func (l *FS) pruneBackups(dir string) error {
	names, err := l.fs.ReadDirNames(dir)
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names[:max(0, len(names)-backupsKept)] {
		if err := l.fs.Remove(l.fs.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}
