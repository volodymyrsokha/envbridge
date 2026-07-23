// Package state persists .envbridge/state.json — the per-environment base
// snapshot (blob and plaintext hashes at last pull/push) that makes drift
// detection three-way.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const dirName = ".envbridge"

type EnvState struct {
	BaseBlobSHA256      string    `json:"base_blob_sha256"`
	BasePlaintextSHA256 string    `json:"base_plaintext_sha256"`
	PulledAt            time.Time `json:"pulled_at"`
}

type State struct {
	Version int                 `json:"version"`
	Envs    map[string]EnvState `json:"envs"`
}

func Load(projectRoot string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, dirName, "state.json"))
	if os.IsNotExist(err) {
		return &State{Version: 1, Envs: map[string]EnvState{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Envs == nil {
		s.Envs = map[string]EnvState{}
	}
	return &s, nil
}

func (s *State) Save(projectRoot string) error {
	dir := filepath.Join(projectRoot, dirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "state.json"))
}
