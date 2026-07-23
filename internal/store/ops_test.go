package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func newTestSession(t *testing.T) (*Session, string) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	l, root := newTestLocal(t)
	return &Session{
		Store:       l,
		Identity:    id,
		Recipients:  []string{id.Recipient().String()},
		UpdatedBy:   "test@local",
		ToolVersion: "test",
	}, root
}

func TestSessionWriteAndCurrent(t *testing.T) {
	sess, root := newTestSession(t)
	ctx := context.Background()
	mat := filepath.Join(root, ".env")
	plaintext := []byte("API_KEY=secret\n")

	m, err := sess.Write(ctx, "prod", plaintext, mat)
	if err != nil {
		t.Fatal(err)
	}
	if m.PlaintextSHA256 != SHA256Hex(plaintext) {
		t.Error("manifest plaintext hash mismatch")
	}

	materialized, err := os.ReadFile(mat)
	if err != nil {
		t.Fatal(err)
	}
	if string(materialized) != string(plaintext) {
		t.Errorf("materialized = %q, want %q", materialized, plaintext)
	}

	gotManifest, got, err := sess.Current(ctx, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest == nil || string(got) != string(plaintext) {
		t.Errorf("Current = %v, %q; want manifest, %q", gotManifest, got, plaintext)
	}
}

func TestSessionCurrentMissingEnv(t *testing.T) {
	sess, _ := newTestSession(t)
	m, plaintext, err := sess.Current(context.Background(), "nope")
	if m != nil || plaintext != nil || err != nil {
		t.Errorf("Current(missing) = %v, %q, %v; want nil, nil, nil", m, plaintext, err)
	}
}

func TestHandEditDrift(t *testing.T) {
	sess, root := newTestSession(t)
	ctx := context.Background()
	mat := filepath.Join(root, ".env")

	m, err := sess.Write(ctx, "prod", []byte("A=1\n"), mat)
	if err != nil {
		t.Fatal(err)
	}

	drifted, _, err := sess.HandEditDrift(ctx, "prod", m)
	if err != nil || drifted {
		t.Errorf("fresh write reported drift: %v, %v", drifted, err)
	}

	if err := os.WriteFile(mat, []byte("A=1\nHOTFIX=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	drifted, materialized, err := sess.HandEditDrift(ctx, "prod", m)
	if err != nil {
		t.Fatal(err)
	}
	if !drifted {
		t.Error("hand-edit not detected")
	}
	if string(materialized) != "A=1\nHOTFIX=1\n" {
		t.Errorf("drift content = %q", materialized)
	}

	os.Remove(mat)
	drifted, _, err = sess.HandEditDrift(ctx, "prod", m)
	if err != nil || drifted {
		t.Errorf("missing materialized file should not be drift: %v, %v", drifted, err)
	}
}

func TestRecipientsFingerprintOrderIndependent(t *testing.T) {
	a := RecipientsFingerprint([]string{"age1aaa", "age1bbb"})
	b := RecipientsFingerprint([]string{"age1bbb", "age1aaa"})
	if a != b {
		t.Error("fingerprint depends on recipient order")
	}
	c := RecipientsFingerprint([]string{"age1aaa"})
	if a == c {
		t.Error("fingerprint ignores roster contents")
	}
}
