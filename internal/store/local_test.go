package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestLocal(t *testing.T) (*Local, string) {
	t.Helper()
	root := t.TempDir()
	l := NewLocal(filepath.Join(root, "store"))
	if err := l.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return l, root
}

func testManifest(materialize string) *Manifest {
	return &Manifest{
		Version:         1,
		BlobSHA256:      SHA256Hex([]byte("blob")),
		PlaintextSHA256: SHA256Hex([]byte("plain")),
		MaterializePath: materialize,
		UpdatedBy:       "test@local",
		UpdatedAt:       time.Now().UTC(),
	}
}

func TestReadMissingEnv(t *testing.T) {
	l, _ := newTestLocal(t)
	ctx := context.Background()
	if _, err := l.ReadManifest(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadManifest err = %v, want ErrNotFound", err)
	}
	if _, err := l.ReadBlob(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadBlob err = %v, want ErrNotFound", err)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	l, root := newTestLocal(t)
	ctx := context.Background()
	mat := filepath.Join(root, "app", ".env")
	os.MkdirAll(filepath.Dir(mat), 0o755)

	m := testManifest(mat)
	if err := l.WriteBlob(ctx, "prod", []byte("blob"), m); err != nil {
		t.Fatal(err)
	}
	if err := l.WriteMaterialized(ctx, "prod", []byte("plain")); err != nil {
		t.Fatal(err)
	}

	got, err := l.ReadManifest(ctx, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.BlobSHA256 != m.BlobSHA256 {
		t.Errorf("manifest blob hash = %s, want %s", got.BlobSHA256, m.BlobSHA256)
	}
	blob, err := l.ReadBlob(ctx, "prod")
	if err != nil || string(blob) != "blob" {
		t.Errorf("ReadBlob = %q, %v", blob, err)
	}
	materialized, err := l.ReadMaterialized(ctx, "prod")
	if err != nil || string(materialized) != "plain" {
		t.Errorf("ReadMaterialized = %q, %v", materialized, err)
	}
	info, err := os.Stat(mat)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("materialized mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestBackupOnOverwriteAndPrune(t *testing.T) {
	l, root := newTestLocal(t)
	ctx := context.Background()
	mat := filepath.Join(root, ".env")

	for i := 0; i < backupsKept+3; i++ {
		blob := []byte(strings.Repeat("x", i+1))
		if err := l.WriteBlob(ctx, "prod", blob, testManifest(mat)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(l.backupDir("prod"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != backupsKept {
		t.Errorf("backups kept = %d, want %d", len(entries), backupsKept)
	}
	newest := entries[len(entries)-1].Name()
	data, err := os.ReadFile(filepath.Join(l.backupDir("prod"), newest))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != backupsKept+2 {
		t.Errorf("newest backup holds blob of len %d, want %d (the previous version)", len(data), backupsKept+2)
	}
}

func TestLockContention(t *testing.T) {
	l, _ := newTestLocal(t)
	ctx := context.Background()

	unlock, err := l.Lock(ctx, "prod", LockInfo{Who: "alice@dev", At: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.Lock(ctx, "prod", LockInfo{Who: "bob@dev", At: time.Now().UTC()})
	if err == nil {
		t.Fatal("second Lock succeeded while held")
	}
	if !strings.Contains(err.Error(), "alice@dev") {
		t.Errorf("lock error does not name the holder: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock2, err := l.Lock(ctx, "prod", LockInfo{Who: "bob@dev", At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("Lock after unlock failed: %v", err)
	}
	unlock2()
}

func TestStaleLockTakeover(t *testing.T) {
	l, _ := newTestLocal(t)
	ctx := context.Background()

	_, err := l.Lock(ctx, "prod", LockInfo{Who: "ghost@gone", At: time.Now().UTC().Add(-lockStaleAfter - time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := l.Lock(ctx, "prod", LockInfo{Who: "alice@dev", At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("stale lock not taken over: %v", err)
	}
	unlock()
}

func TestLocksAreIndependentPerEnv(t *testing.T) {
	l, _ := newTestLocal(t)
	ctx := context.Background()
	unlockA, err := l.Lock(ctx, "prod", LockInfo{Who: "a", At: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	defer unlockA()
	unlockB, err := l.Lock(ctx, "staging", LockInfo{Who: "b", At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("staging lock blocked by prod lock: %v", err)
	}
	unlockB()
}
