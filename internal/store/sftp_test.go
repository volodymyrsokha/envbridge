package store

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

// startSSHServer runs an in-process SSH server with an SFTP subsystem
// serving the test host's filesystem, accepting any authentication.
func startSSHServer(t *testing.T) string {
	t.Helper()
	srv := &gliderssh.Server{
		Handler: func(s gliderssh.Session) {},
		SubsystemHandlers: map[string]gliderssh.SubsystemHandler{
			"sftp": func(sess gliderssh.Session) {
				server, err := sftp.NewServer(sess)
				if err != nil {
					return
				}
				server.Serve()
			},
		},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func dialTestSFTP(t *testing.T, addr string) *sftp.Client {
	t.Helper()
	sshClient, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User:            "test",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sshClient.Close() })
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sftpClient.Close() })
	return sftpClient
}

func newTestSFTPStore(t *testing.T) (*FS, string) {
	t.Helper()
	addr := startSSHServer(t)
	client := dialTestSFTP(t, addr)
	root := t.TempDir()
	s := NewSFTP(client, filepath.Join(root, "store"))
	if err := s.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, root
}

func TestSFTPWriteReadRoundTrip(t *testing.T) {
	s, root := newTestSFTPStore(t)
	ctx := context.Background()
	mat := filepath.Join(root, ".env")

	if err := s.WriteBlob(ctx, "prod", []byte("blob"), testManifest(mat)); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteMaterialized(ctx, "prod", []byte("plain")); err != nil {
		t.Fatal(err)
	}

	blob, err := s.ReadBlob(ctx, "prod")
	if err != nil || string(blob) != "blob" {
		t.Errorf("ReadBlob = %q, %v", blob, err)
	}
	materialized, err := s.ReadMaterialized(ctx, "prod")
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

func TestSFTPOverwriteKeepsBackup(t *testing.T) {
	s, root := newTestSFTPStore(t)
	ctx := context.Background()
	mat := filepath.Join(root, ".env")

	for i := 0; i < 3; i++ {
		if err := s.WriteBlob(ctx, "prod", []byte(strings.Repeat("x", i+1)), testManifest(mat)); err != nil {
			t.Fatal(err)
		}
	}
	names, err := os.ReadDir(s.backupDir("prod"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("backups = %d, want 2", len(names))
	}
	blob, err := s.ReadBlob(ctx, "prod")
	if err != nil || len(blob) != 3 {
		t.Errorf("current blob len = %d, %v; want 3", len(blob), err)
	}
}

func TestSFTPLockContention(t *testing.T) {
	s, _ := newTestSFTPStore(t)
	ctx := context.Background()

	unlock, err := s.Lock(ctx, "prod", LockInfo{Who: "alice@dev", At: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Lock(ctx, "prod", LockInfo{Who: "bob@dev", At: time.Now().UTC()})
	if err == nil {
		t.Fatal("second Lock succeeded while held")
	}
	if !strings.Contains(err.Error(), "alice@dev") {
		t.Errorf("lock error does not name the holder over sftp: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock2, err := s.Lock(ctx, "prod", LockInfo{Who: "bob@dev", At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("Lock after unlock failed: %v", err)
	}
	unlock2()
}

func TestSFTPLockBootstrapsFreshStore(t *testing.T) {
	addr := startSSHServer(t)
	client := dialTestSFTP(t, addr)
	s := NewSFTP(client, filepath.Join(t.TempDir(), "brand", "new", "store"))

	unlock, err := s.Lock(context.Background(), "prod", LockInfo{Who: "first@push", At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("Lock on fresh server failed: %v", err)
	}
	unlock()
}

// startSSHServerAt serves SFTP with the given working directory, so tilde
// paths resolve under it (Getwd on a fresh session returns the working dir).
func startSSHServerAt(t *testing.T, workDir string) string {
	t.Helper()
	srv := &gliderssh.Server{
		Handler: func(s gliderssh.Session) {},
		SubsystemHandlers: map[string]gliderssh.SubsystemHandler{
			"sftp": func(sess gliderssh.Session) {
				server, err := sftp.NewServer(sess, sftp.WithServerWorkingDirectory(workDir))
				if err != nil {
					return
				}
				server.Serve()
			},
		},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func TestSFTPTildeExpansion(t *testing.T) {
	home := t.TempDir()
	addr := startSSHServerAt(t, home)
	client := dialTestSFTP(t, addr)

	s := NewSFTP(client, "~/envbridge-store")
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBlob(ctx, "prod", []byte("blob"), testManifest("~/app/.env")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteMaterialized(ctx, "prod", []byte("plain")); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(home, "envbridge-store", "prod.env.age")); err != nil {
		t.Errorf("blob not under expanded home: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, "app", ".env"))
	if err != nil || string(data) != "plain" {
		t.Errorf("materialized under expanded home = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, "~")); err == nil {
		t.Error("a literal ~ directory was created")
	}

	got, err := s.ReadPath(ctx, "~/app/.env")
	if err != nil || string(got) != "plain" {
		t.Errorf("ReadPath through tilde = %q, %v", got, err)
	}
}
