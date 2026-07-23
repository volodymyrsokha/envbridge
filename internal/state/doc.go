// Package state persists .envbridge/state.json — the per-environment base
// snapshot (blob and plaintext hashes at last pull/push) that makes drift
// detection three-way. Implemented in M3.
package state
