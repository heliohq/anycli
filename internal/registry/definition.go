package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shipbase/anycli/internal/config"
)

// Definition represents a wrapper definition for a CLI tool.
type Definition struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Binary      string        `json:"binary"`
	Resolve     string        `json:"resolve,omitempty"` // "which" (default) or absolute path
	Source      *SourceConfig `json:"source,omitempty"`
	Auth        *AuthConfig   `json:"auth,omitempty"`
	Before      []Rule        `json:"before,omitempty"`
	After       []Rule        `json:"after,omitempty"`
}

// SourceConfig defines how to download the real CLI binary.
type SourceConfig struct {
	Type         string            `json:"type"`                    // "github-release"
	Repo         string            `json:"repo"`                    // e.g. "cli/cli"
	AssetPattern string            `json:"asset_pattern"`           // e.g. "gh_{version}_{os}_{arch}{ext}"
	BinaryPath   string            `json:"binary_path"`             // path inside archive, e.g. "gh_{version}_{os}_{arch}/bin/gh"
	Version      string            `json:"version,omitempty"`       // pinned version, empty = latest
	OsMap        map[string]string `json:"os_map,omitempty"`        // Go GOOS -> release naming, e.g. {"darwin":"macOS"}
	ExtMap       map[string]string `json:"ext_map,omitempty"`       // Go GOOS -> file extension, e.g. {"darwin":".zip"}
}

// AuthConfig defines how to authenticate for a tool.
type AuthConfig struct {
	Type   string `json:"type"`    // "env", "token"
	EnvVar string `json:"env_var"` // environment variable name
	Prompt string `json:"prompt"`  // prompt text for anycli auth
}

// Rule represents a single before/after middleware rule.
type Rule struct {
	Name   string                 `json:"name"`
	Rule   string                 `json:"rule"`
	When   map[string]interface{} `json:"when,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// Unmarshal parses JSON data into a Definition.
func Unmarshal(data []byte, def *Definition) error {
	return json.Unmarshal(data, def)
}

// Load reads a wrapper definition from the local registry.
func Load(name string) (*Definition, error) {
	path := filepath.Join(config.RegistryDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wrapper %q not installed: %w", name, err)
	}

	var def Definition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("invalid wrapper definition for %q: %w", name, err)
	}
	return &def, nil
}

// Save writes a wrapper definition to the local registry.
func Save(def *Definition) error {
	if err := config.EnsureDirs(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(config.RegistryDir(), def.Name+".json")
	return os.WriteFile(path, data, 0644)
}

// List returns all installed wrapper names.
func List() ([]string, error) {
	dir := config.RegistryDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext == ".json" {
			names = append(names, e.Name()[:len(e.Name())-len(ext)])
		}
	}
	return names, nil
}

// Remove deletes a wrapper definition from the local registry.
func Remove(name string) error {
	path := filepath.Join(config.RegistryDir(), name+".json")
	return os.Remove(path)
}
