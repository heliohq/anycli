package credential

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shipbase/anycli/internal/registry"
)

func TestResolve_StandaloneMode(t *testing.T) {
	home := setupHome(t)
	// Clear vault env vars to ensure standalone mode
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	// Create local credential file
	credDir := filepath.Join(home, "credentials")
	if err := os.MkdirAll(credDir, 0755); err != nil {
		t.Fatalf("failed to create credentials dir: %v", err)
	}
	creds := map[string]string{
		"GH_TOKEN": "ghp_local_resolve",
		"API_KEY":  "key_local_resolve",
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(filepath.Join(credDir, "gh.json"), data, 0600); err != nil {
		t.Fatalf("failed to write cred file: %v", err)
	}

	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{LocalKey: "GH_TOKEN"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "GITHUB_TOKEN"},
		},
		{
			Source: registry.CredentialSource{LocalKey: "API_KEY"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "MY_API_KEY"},
		},
	}

	values, err := Resolve("gh", bindings)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("values length = %d, want 2", len(values))
	}
	if values[0] != "ghp_local_resolve" {
		t.Errorf("values[0] = %q, want %q", values[0], "ghp_local_resolve")
	}
	if values[1] != "key_local_resolve" {
		t.Errorf("values[1] = %q, want %q", values[1], "key_local_resolve")
	}
}

func TestResolve_MissingCredentials(t *testing.T) {
	setupHome(t)
	// Clear vault env vars
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{LocalKey: "NONEXISTENT_KEY"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "TOKEN"},
		},
	}

	values, err := Resolve("no-such-tool", bindings)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("values length = %d, want 1", len(values))
	}
	// Missing credentials should return empty strings
	if values[0] != "" {
		t.Errorf("values[0] = %q, want empty string", values[0])
	}
}

func TestResolve_EmptyBindings(t *testing.T) {
	setupHome(t)
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	values, err := Resolve("tool", nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if values != nil {
		t.Errorf("values = %v, want nil for empty bindings", values)
	}
}

func TestGetVaultConfig_NoEnvVars(t *testing.T) {
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	cfg, err := GetVaultConfig()
	if err != nil {
		t.Fatalf("GetVaultConfig returned error: %v", err)
	}
	if cfg != nil {
		t.Errorf("GetVaultConfig returned non-nil config when no env vars set: %+v", cfg)
	}
}

func TestGetVaultConfig_AllEnvVars(t *testing.T) {
	t.Setenv("ANYCLI_VAULT_URL", "https://vault.example.com")
	t.Setenv("ANYCLI_VAULT_TOKEN", "vault-tok-abc")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "ws-456")

	cfg, err := GetVaultConfig()
	if err != nil {
		t.Fatalf("GetVaultConfig returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("GetVaultConfig returned nil")
	}
	if cfg.URL != "https://vault.example.com" {
		t.Errorf("URL = %q, want %q", cfg.URL, "https://vault.example.com")
	}
	if cfg.Token != "vault-tok-abc" {
		t.Errorf("Token = %q, want %q", cfg.Token, "vault-tok-abc")
	}
	if cfg.WorkspaceID != "ws-456" {
		t.Errorf("WorkspaceID = %q, want %q", cfg.WorkspaceID, "ws-456")
	}
}

func TestGetVaultConfig_PartialEnvVars(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		token       string
		workspaceID string
		wantMissing string
	}{
		{
			name:        "missing token and workspace",
			url:         "https://vault.example.com",
			token:       "",
			workspaceID: "",
			wantMissing: "ANYCLI_VAULT_TOKEN",
		},
		{
			name:        "missing url",
			url:         "",
			token:       "tok",
			workspaceID: "ws-1",
			wantMissing: "ANYCLI_VAULT_URL",
		},
		{
			name:        "missing workspace_id",
			url:         "https://vault.example.com",
			token:       "tok",
			workspaceID: "",
			wantMissing: "ANYCLI_VAULT_WORKSPACE_ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANYCLI_VAULT_URL", tt.url)
			t.Setenv("ANYCLI_VAULT_TOKEN", tt.token)
			t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", tt.workspaceID)

			cfg, err := GetVaultConfig()
			if err == nil {
				t.Fatalf("GetVaultConfig should return error for partial config, got config: %+v", cfg)
			}
			if !strings.Contains(err.Error(), tt.wantMissing) {
				t.Errorf("error should mention %q, got: %v", tt.wantMissing, err)
			}
		})
	}
}

func TestIsVaultMode(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		token       string
		workspaceID string
		want        bool
	}{
		{
			name:        "all set",
			url:         "https://vault.example.com",
			token:       "tok",
			workspaceID: "ws-1",
			want:        true,
		},
		{
			name:        "none set",
			url:         "",
			token:       "",
			workspaceID: "",
			want:        false,
		},
		{
			name:        "partial",
			url:         "https://vault.example.com",
			token:       "",
			workspaceID: "ws-1",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANYCLI_VAULT_URL", tt.url)
			t.Setenv("ANYCLI_VAULT_TOKEN", tt.token)
			t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", tt.workspaceID)

			got := IsVaultMode()
			if got != tt.want {
				t.Errorf("IsVaultMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
