package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ProjectFileName = ".envbridge.yaml"

type Project struct {
	Version      int                    `yaml:"version"`
	Project      string                 `yaml:"project"`
	Store        string                 `yaml:"store"`
	Environments map[string]Environment `yaml:"environments"`
	Recipients   []Recipient            `yaml:"recipients"`
}

type Environment struct {
	Host        string `yaml:"host"`
	Materialize string `yaml:"materialize"`
	Local       string `yaml:"local"`
	Store       string `yaml:"store,omitempty"`
}

type Recipient struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	Key   string `yaml:"key"`
}

// Discover walks up from start looking for .envbridge.yaml, git-style.
// Returns the parsed project and the directory containing the file.
func Discover(start string) (*Project, string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, "", err
	}
	for {
		path := filepath.Join(dir, ProjectFileName)
		if _, err := os.Stat(path); err == nil {
			p, err := LoadProject(path)
			if err != nil {
				return nil, "", err
			}
			return p, dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, "", fmt.Errorf("no %s found in this or any parent directory — run `envbridge init` to create a project", ProjectFileName)
		}
		dir = parent
	}
}

func LoadProject(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Project
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &p, nil
}

func (p *Project) Validate() error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported config version %d (this envbridge understands version 1)", p.Version)
	}
	if p.Project == "" {
		return fmt.Errorf("project name is required")
	}
	if p.Store == "" {
		return fmt.Errorf("store path is required")
	}
	if len(p.Environments) == 0 {
		return fmt.Errorf("at least one environment is required")
	}
	for name, env := range p.Environments {
		if env.Host == "" {
			return fmt.Errorf("environment %q: host is required", name)
		}
		if env.Materialize == "" {
			return fmt.Errorf("environment %q: materialize path is required", name)
		}
		if env.Local == "" {
			return fmt.Errorf("environment %q: local path is required", name)
		}
	}
	if len(p.Recipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	for _, r := range p.Recipients {
		if !strings.HasPrefix(r.Key, "age1") {
			return fmt.Errorf("recipient %q: key %q does not look like an age public key", r.Name, r.Key)
		}
	}
	return nil
}

// StoreFor returns the store root for an environment, honoring the per-env
// override.
func (p *Project) StoreFor(env string) string {
	if e, ok := p.Environments[env]; ok && e.Store != "" {
		return e.Store
	}
	return p.Store
}

func (p *Project) RecipientKeys() []string {
	keys := make([]string, 0, len(p.Recipients))
	for _, r := range p.Recipients {
		keys = append(keys, r.Key)
	}
	return keys
}

// EnvNames returns environment names sorted for stable output.
func (p *Project) EnvNames() []string {
	names := make([]string, 0, len(p.Environments))
	for name := range p.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
