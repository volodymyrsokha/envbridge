package cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	gliderssh "github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"

	"github.com/volodymyrsokha/envbridge/internal/agecrypt"
	"github.com/volodymyrsokha/envbridge/internal/config"
	"github.com/volodymyrsokha/envbridge/internal/sshx"
	"github.com/volodymyrsokha/envbridge/internal/state"
	"github.com/volodymyrsokha/envbridge/internal/store"
)

func startSSHServer(t *testing.T) string {
	t.Helper()
	srv := &gliderssh.Server{
		Handler: func(s gliderssh.Session) {},
		SubsystemHandlers: map[string]gliderssh.SubsystemHandler{
			"sftp": func(sess gliderssh.Session) {
				if server, err := sftp.NewServer(sess); err == nil {
					server.Serve()
				}
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

// newTestProject builds a project the way openProject would, but pointed at
// the in-process SSH server and a throwaway identity — one per simulated
// developer clone.
func newTestProject(t *testing.T, addr, serverDir string, identity *age.X25519Identity) *project {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Project{
		Version: 1,
		Project: "cli-test",
		Store:   filepath.Join(serverDir, "store"),
		Environments: map[string]config.Environment{
			"production": {
				Host:        addr,
				Materialize: filepath.Join(serverDir, "srv", ".env"),
				Local:       ".env.production",
			},
		},
		Recipients: []config.Recipient{{Name: "t", Email: "t@t", Key: identity.Recipient().String()}},
	}
	st, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	dialer := &sshx.Dialer{
		GetOption:      func(alias, key string) string { return "" },
		KnownHostsPath: filepath.Join(root, "known_hosts"),
		TrustPrompt:    func(host, fingerprint string) bool { return true },
		Timeout:        5 * time.Second,
	}
	p := &project{
		cfg:      cfg,
		root:     root,
		identity: identity,
		st:       st,
		dialer:   dialer,
		ssh:      map[string]*gossh.Client{},
		sftp:     map[string]*sftp.Client{},
	}
	t.Cleanup(p.Close)
	return p
}

func writeLocal(t *testing.T, p *project, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(p.root, ".env.production"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readLocal(t *testing.T, p *project) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.root, ".env.production"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPushPullConflictAndAdoptOverSSH(t *testing.T) {
	ctx := context.Background()
	addr := startSSHServer(t)
	serverDir := t.TempDir()
	os.MkdirAll(filepath.Join(serverDir, "srv"), 0o755)

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	alice := newTestProject(t, addr, serverDir, identity)
	bob := newTestProject(t, addr, serverDir, identity)

	// Alice creates the environment with the first push.
	writeLocal(t, alice, "A=1\n")
	if err := pushOne(ctx, alice, "production", false); err != nil {
		t.Fatalf("first push: %v", err)
	}
	materialized, err := os.ReadFile(filepath.Join(serverDir, "srv", ".env"))
	if err != nil || string(materialized) != "A=1\n" {
		t.Fatalf("materialized = %q, %v", materialized, err)
	}

	// Bob pulls it and pushes a change.
	if err := pullOne(ctx, bob, "production", false, false); err != nil {
		t.Fatalf("bob pull: %v", err)
	}
	if readLocal(t, bob) != "A=1\n" {
		t.Fatalf("bob local = %q", readLocal(t, bob))
	}
	writeLocal(t, bob, "A=1\nB=2\n")
	if err := pushOne(ctx, bob, "production", false); err != nil {
		t.Fatalf("bob push: %v", err)
	}

	// Alice edits without pulling first — push must detect the conflict.
	writeLocal(t, alice, "A=99\n")
	err = pushOne(ctx, alice, "production", false)
	if err == nil || !strings.Contains(err.Error(), "changed on the server") {
		t.Fatalf("conflicting push: err = %v, want server-changed refusal", err)
	}

	// A hand-edit on the server blocks pushes and is adopted via pull.
	if err := os.WriteFile(filepath.Join(serverDir, "srv", ".env"), []byte("A=1\nB=2\nHOTFIX=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = pushOne(ctx, bob, "production", false)
	if err == nil || !strings.Contains(err.Error(), "hand-edited") {
		t.Fatalf("push over hand-edit: err = %v, want hand-edit refusal", err)
	}
	if err := pullOne(ctx, bob, "production", true, false); err != nil {
		t.Fatalf("pull --adopt: %v", err)
	}
	if got := readLocal(t, bob); got != "A=1\nB=2\nHOTFIX=1\n" {
		t.Fatalf("after adopt, bob local = %q", got)
	}
	if err := pushOne(ctx, bob, "production", false); err == nil {
		t.Log("post-adopt push is a no-op in-sync push")
	} else if !strings.Contains(err.Error(), "in sync") {
		t.Fatalf("post-adopt push: %v", err)
	}

	// Status reflects a clean, synced state for Bob.
	row := statusOne(ctx, bob, "production")
	if row.Error != "" || row.Local != "✓ in sync" || !strings.HasPrefix(row.Server, "✓ clean") {
		t.Fatalf("status row = %+v", row)
	}
}

func TestTeamSyncReencryptsForNewRoster(t *testing.T) {
	ctx := context.Background()
	addr := startSSHServer(t)
	serverDir := t.TempDir()
	os.MkdirAll(filepath.Join(serverDir, "srv"), 0o755)

	identity, _ := age.GenerateX25519Identity()
	alice := newTestProject(t, addr, serverDir, identity)
	writeLocal(t, alice, "SECRET=x\n")
	if err := pushOne(ctx, alice, "production", false); err != nil {
		t.Fatal(err)
	}

	newcomer, _ := age.GenerateX25519Identity()
	alice.cfg.Recipients = append(alice.cfg.Recipients,
		config.Recipient{Name: "New", Email: "new@t", Key: newcomer.Recipient().String()})

	fingerprint := store.RecipientsFingerprint(alice.cfg.RecipientKeys())
	if err := syncOne(ctx, alice, "production", fingerprint); err != nil {
		t.Fatal(err)
	}

	st, err := alice.storeFor("production")
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.ReadManifest(ctx, "production")
	if err != nil {
		t.Fatal(err)
	}
	if m.RecipientsFingerprint != fingerprint {
		t.Error("manifest fingerprint not updated")
	}
	blob, err := st.ReadBlob(ctx, "production")
	if err != nil {
		t.Fatal(err)
	}
	for who, id := range map[string]*age.X25519Identity{"original": identity, "newcomer": newcomer} {
		got, err := agecrypt.Decrypt(blob, id)
		if err != nil || string(got) != "SECRET=x\n" {
			t.Errorf("%s cannot decrypt after sync: %q, %v", who, got, err)
		}
	}

	// Second sync is a no-op for an already-synced roster.
	if err := syncOne(ctx, alice, "production", fingerprint); err != nil {
		t.Fatal(err)
	}
}
