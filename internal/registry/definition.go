package registry

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

// SourceConfig defines how to obtain the real CLI binary.
//
// Types "github-release" and "npm" are declarative metadata only: the host/pod
// image provisions those binaries, AnyCLI never fetches them (the gh pin is
// intentionally decoupled from what the image installs).
//
// Type "direct" additionally enables lazy install: the engine downloads the
// pinned archive from the official URL computed by URLTemplate, verifies the
// mandatory per-platform SHA256, and unpacks it under the pinned-versions
// directory. There is no CDN mirror and no fallback source.
type SourceConfig struct {
	Type         string            `json:"type"`                    // "github-release", "npm", or "direct"
	Repo         string            `json:"repo,omitempty"`          // e.g. "cli/cli" (github-release)
	AssetPattern string            `json:"asset_pattern,omitempty"` // e.g. "gh_{version}_{os}_{arch}{ext}"
	BinaryPath   string            `json:"binary_path,omitempty"`   // path inside archive; {version}/{os}/{arch}/{exe} expand
	Version      string            `json:"version,omitempty"`       // pinned version, empty = latest
	OsMap        map[string]string `json:"os_map,omitempty"`        // Go GOOS -> release naming
	ArchMap      map[string]string `json:"arch_map,omitempty"`      // Go GOARCH -> release naming (e.g. amd64 -> x64)
	ExtMap       map[string]string `json:"ext_map,omitempty"`       // Go GOOS -> archive extension (".tgz" / ".zip")
	URLTemplate  string            `json:"url_template,omitempty"`  // "direct" type: full download URL with {version}/{os}/{arch}/{ext}
	SHA256       map[string]string `json:"sha256,omitempty"`        // "direct" type: "<os>-<arch>" platform key -> hex digest (mandatory)
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

// CredentialSource identifies one scalar value in the resolver-supplied Data
// map. Credential acquisition and persistence belong to the host.
type CredentialSource struct {
	Field string `json:"field"`
}

// CredentialInject specifies how to deliver the credential to the tool.
type CredentialInject struct {
	Type       string            `json:"type"`                  // "env", "arg", "file"
	EnvVar     string            `json:"env_var,omitempty"`     // for type "env"
	Flag       string            `json:"flag,omitempty"`        // for type "arg"
	Format     string            `json:"format,omitempty"`      // for type "arg": "" (space-separated) or "eq" (=)
	Path       string            `json:"path,omitempty"`        // for type "file": template config path
	ConfigEnv  string            `json:"config_env,omitempty"`  // for type "file": env var to redirect config path
	ConfigFlag string            `json:"config_flag,omitempty"` // for type "file": flag to redirect config path
	FileFormat string            `json:"file_format,omitempty"` // for type "file": "yaml", "json", "toml", "ini", "custom"
	Fields     map[string]string `json:"fields,omitempty"`      // for type "file": dot-path -> template
	Patcher    string            `json:"patcher,omitempty"`     // for type "file" with file_format "custom"
	Mode       string            `json:"mode,omitempty"`        // for type "file": file permission, default "0600"
}

// Rule represents a single before/after middleware rule.
type Rule struct {
	Name   string                 `json:"name"`
	Rule   string                 `json:"rule"`
	When   map[string]interface{} `json:"when,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
}
