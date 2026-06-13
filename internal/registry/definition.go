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

// SourceConfig defines how to download the real CLI binary. It is retained as
// declarative metadata on a definition; in embedded use the host/pod image
// provisions the underlying binaries rather than AnyCLI fetching them.
type SourceConfig struct {
	Type         string            `json:"type"`              // "github-release" or "npm"
	Repo         string            `json:"repo"`              // e.g. "cli/cli"
	AssetPattern string            `json:"asset_pattern"`     // e.g. "gh_{version}_{os}_{arch}{ext}"
	BinaryPath   string            `json:"binary_path"`       // path inside archive
	Version      string            `json:"version,omitempty"` // pinned version, empty = latest
	OsMap        map[string]string `json:"os_map,omitempty"`  // Go GOOS -> release naming
	ExtMap       map[string]string `json:"ext_map,omitempty"` // Go GOOS -> file extension
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

// CredentialSource specifies where to find the credential value within the
// resolver-supplied Data map.
type CredentialSource struct {
	VaultTool  string `json:"vault_tool,omitempty"`  // Tool name in vault (e.g., "github")
	VaultField string `json:"vault_field,omitempty"` // Field key in the credential Data (e.g., "access_token")
	LocalKey   string `json:"local_key"`             // Key in local credential file (e.g., "GH_TOKEN")
	AuthFlag   string `json:"auth_flag,omitempty"`   // CLI flag name for a future `auth --set` shell
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
