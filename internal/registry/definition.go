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
	Type        string        `json:"type,omitempty"` // "" (default, = "cli") or "service"
	Description string        `json:"description"`
	Binary      string        `json:"binary,omitempty"`
	Resolve     string        `json:"resolve,omitempty"` // "which" (default) or absolute path
	Source      *SourceConfig `json:"source,omitempty"`
	Auth        *AuthConfig   `json:"auth,omitempty"`
	Before      []Rule        `json:"before,omitempty"`
	After       []Rule        `json:"after,omitempty"`
}

// SourceConfig defines how to download the real CLI binary.
type SourceConfig struct {
	Type         string            `json:"type"`                    // "github-release" or "npm"
	Repo         string            `json:"repo"`                    // e.g. "cli/cli"
	AssetPattern string            `json:"asset_pattern"`           // e.g. "gh_{version}_{os}_{arch}{ext}"
	BinaryPath   string            `json:"binary_path"`             // path inside archive, e.g. "gh_{version}_{os}_{arch}/bin/gh"
	Version      string            `json:"version,omitempty"`       // pinned version, empty = latest
	OsMap        map[string]string `json:"os_map,omitempty"`        // Go GOOS -> release naming, e.g. {"darwin":"macOS"}
	ExtMap       map[string]string `json:"ext_map,omitempty"`       // Go GOOS -> file extension, e.g. {"darwin":".zip"}
}

// AuthConfig defines how to authenticate for a tool.
type AuthConfig struct {
	Credentials []CredentialBinding `json:"credentials,omitempty"`
}

// CredentialBinding pairs a credential source with an injection method.
type CredentialBinding struct {
	Source CredentialSource `json:"source"`
	Inject CredentialInject `json:"inject"`
}

// CredentialSource specifies where to find the credential value.
type CredentialSource struct {
	VaultTool  string `json:"vault_tool,omitempty"`  // Tool name in vault (e.g., "github")
	VaultField string `json:"vault_field,omitempty"` // Field path in vault data JSON (e.g., "access_token")
	LocalKey   string `json:"local_key"`             // Key in local credential file (e.g., "GH_TOKEN")
	AuthFlag   string `json:"auth_flag,omitempty"`   // CLI flag name for `any auth --set` (e.g., "token")
}

// CredentialInject specifies how to deliver the credential to the tool.
type CredentialInject struct {
	Type       string            `json:"type"`                   // "env", "arg", "file"
	EnvVar     string            `json:"env_var,omitempty"`      // for type "env"
	Flag       string            `json:"flag,omitempty"`         // for type "arg"
	Format     string            `json:"format,omitempty"`       // for type "arg": "" (space-separated) or "eq" (=)
	Path       string            `json:"path,omitempty"`         // for type "file": target file path
	ConfigEnv  string            `json:"config_env,omitempty"`   // for type "file" in vault mode: env var to override config path
	ConfigFlag string            `json:"config_flag,omitempty"`  // for type "file" in vault mode: flag to override config path
	FileFormat string            `json:"file_format,omitempty"`  // for type "file": "yaml", "json", "toml", "ini", "custom"
	Fields     map[string]string `json:"fields,omitempty"`       // for type "file": dot-path -> template
	Patcher    string            `json:"patcher,omitempty"`      // for type "file" with file_format "custom"
	Mode       string            `json:"mode,omitempty"`         // for type "file": file permission, default "0600"
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

	// Migrate legacy auth schema if present
	var legacy legacyAuthDetector
	if err := json.Unmarshal(data, &legacy); err == nil && legacy.Auth != nil {
		if def.Auth != nil && len(def.Auth.Credentials) == 0 && legacy.Auth.Type != "" {
			// Legacy definition detected — migrate in memory
			switch legacy.Auth.Type {
			case "managed":
				if legacy.Auth.EnvVar != "" {
					def.Auth.Credentials = []CredentialBinding{{
						Source: CredentialSource{LocalKey: legacy.Auth.EnvVar},
						Inject: CredentialInject{Type: "env", EnvVar: legacy.Auth.EnvVar},
					}}
				}
			case "self":
				// Legacy self-managed auth is no longer supported.
				// The tool's own auth mechanism still works (e.g., wrangler stores its own tokens),
				// but AnyCLI no longer delegates to the tool's login command.
				// Clear auth so AnyCLI doesn't interfere.
				fmt.Fprintf(os.Stderr, "[anycli] warning: tool %q uses legacy 'self' auth type which is no longer supported; auth cleared\n", name)
				def.Auth = nil
			}
		}
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

// legacyAuthDetector is used to detect and migrate legacy auth schema fields.
type legacyAuthDetector struct {
	Auth *struct {
		Type    string `json:"type"`
		EnvVar  string `json:"env_var"`
		Command string `json:"command"`
	} `json:"auth"`
}

// Remove deletes a wrapper definition from the local registry.
func Remove(name string) error {
	path := filepath.Join(config.RegistryDir(), name+".json")
	return os.Remove(path)
}
