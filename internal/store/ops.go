package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/volodymyrsokha/envbridge/internal/agecrypt"
)

// Session bundles the identity and team context every store operation needs.
type Session struct {
	Store       Store
	Identity    *age.X25519Identity
	Recipients  []string
	UpdatedBy   string
	ToolVersion string
}

// Current returns the manifest and decrypted plaintext for env, or
// (nil, nil, nil) when the environment doesn't exist in the store yet.
func (s *Session) Current(ctx context.Context, env string) (*Manifest, []byte, error) {
	m, err := s.Store.ReadManifest(ctx, env)
	if errors.Is(err, ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	blob, err := s.Store.ReadBlob(ctx, env)
	if err != nil {
		return nil, nil, err
	}
	if got := SHA256Hex(blob); got != m.BlobSHA256 {
		return nil, nil, fmt.Errorf("store corruption for %q: blob hash %s does not match manifest %s", env, got[:8], m.BlobSHA256[:8])
	}
	plaintext, err := agecrypt.Decrypt(blob, s.Identity)
	if err != nil {
		return nil, nil, err
	}
	return m, plaintext, nil
}

// HandEditDrift reports whether the materialized file diverged from what the
// manifest says was last written — i.e. someone edited it outside envbridge.
// A missing materialized file counts as no drift (it will be re-created on
// the next write).
func (s *Session) HandEditDrift(ctx context.Context, env string, m *Manifest) (bool, []byte, error) {
	materialized, err := s.Store.ReadMaterialized(ctx, env)
	if errors.Is(err, ErrNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if SHA256Hex(materialized) == m.PlaintextSHA256 {
		return false, materialized, nil
	}
	return true, materialized, nil
}

// Write encrypts plaintext to the session's recipients and runs the full
// pipeline: blob backup + atomic swap + manifest, then materialization.
// Callers must hold the env's lock.
func (s *Session) Write(ctx context.Context, env string, plaintext []byte, materialize string) (*Manifest, error) {
	blob, err := agecrypt.Encrypt(plaintext, s.Recipients)
	if err != nil {
		return nil, err
	}
	m := &Manifest{
		Version:               1,
		BlobSHA256:            SHA256Hex(blob),
		PlaintextSHA256:       SHA256Hex(plaintext),
		MaterializePath:       materialize,
		RecipientsFingerprint: RecipientsFingerprint(s.Recipients),
		UpdatedBy:             s.UpdatedBy,
		UpdatedAt:             time.Now().UTC(),
		ToolVersion:           s.ToolVersion,
	}
	if err := s.Store.WriteBlob(ctx, env, blob, m); err != nil {
		return nil, err
	}
	if err := s.Store.WriteMaterialized(ctx, env, plaintext); err != nil {
		return nil, err
	}
	return m, nil
}

func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// RecipientsFingerprint hashes the sorted recipient keys so manifests can
// record which roster a blob was encrypted for.
func RecipientsFingerprint(keys []string) string {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	return SHA256Hex([]byte(strings.Join(sorted, "\n")))
}
