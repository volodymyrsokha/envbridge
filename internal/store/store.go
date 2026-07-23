// Package store defines the transport seam between commands and the
// server-side env store. localStore serves the same binary running on the
// server; sftpStore serves dev machines over SSH; any v2 transport (daemon,
// ssh-exec fallback) is a third implementation of the same interface.
package store

import (
	"context"
	"time"
)

type Manifest struct {
	Version               int       `json:"version"`
	BlobSHA256            string    `json:"blob_sha256"`
	PlaintextSHA256       string    `json:"plaintext_sha256"`
	MaterializePath       string    `json:"materialize_path"`
	RecipientsFingerprint string    `json:"recipients_fingerprint"`
	UpdatedBy             string    `json:"updated_by"`
	UpdatedAt             time.Time `json:"updated_at"`
	ToolVersion           string    `json:"tool_version"`
}

type LockInfo struct {
	Who  string    `json:"who"`
	Host string    `json:"host"`
	At   time.Time `json:"at"`
}

type Store interface {
	ReadManifest(ctx context.Context, env string) (*Manifest, error)
	ReadBlob(ctx context.Context, env string) ([]byte, error)
	// WriteBlob performs backup, atomic rename, and manifest update as one
	// logical operation.
	WriteBlob(ctx context.Context, env string, blob []byte, m *Manifest) error
	ReadMaterialized(ctx context.Context, env string) ([]byte, error)
	WriteMaterialized(ctx context.Context, env string, plaintext []byte) error
	Lock(ctx context.Context, env string, info LockInfo) (unlock func() error, err error)
	Init(ctx context.Context) error
}
