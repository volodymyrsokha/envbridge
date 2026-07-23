// Package config loads, validates, and saves envbridge configuration: the
// committed .envbridge.yaml (with git-style upward discovery) and the user's
// local settings under ~/.config/envbridge.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Local holds per-user settings from ~/.config/envbridge/config.yaml. All
// fields are optional.
type Local struct {
	Identity string `yaml:"identity"`
	Editor   string `yaml:"editor"`
}

func userConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "envbridge"), nil
}

func LoadLocal() (*Local, error) {
	dir, err := userConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if os.IsNotExist(err) {
		return &Local{}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Local
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// IdentityPath resolves the age identity location: ENVBRIDGE_IDENTITY wins,
// then the local config's identity field, then the default
// ~/.config/envbridge/identity.txt.
func IdentityPath() (string, error) {
	if p := os.Getenv("ENVBRIDGE_IDENTITY"); p != "" {
		return p, nil
	}
	l, err := LoadLocal()
	if err != nil {
		return "", err
	}
	if l.Identity != "" {
		return l.Identity, nil
	}
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.txt"), nil
}

// Editor resolves the editor command: local config, then $VISUAL, then
// $EDITOR, then vi.
func Editor() string {
	if l, err := LoadLocal(); err == nil && l.Editor != "" {
		return l.Editor
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}
