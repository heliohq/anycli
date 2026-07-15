package credential

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/registry"
)

func TestApplyBindings_EnvInjection(t *testing.T) {
	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "access_token"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "GITHUB_TOKEN"},
		},
		{
			Source: registry.CredentialSource{Field: "api_key"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "MY_API_KEY"},
		},
	}
	values := []string{"ghp_abc123", "key_xyz"}

	result, err := ApplyBindings("test-tool", bindings, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}

	if result.Env["GITHUB_TOKEN"] != "ghp_abc123" {
		t.Errorf("GITHUB_TOKEN = %q, want %q", result.Env["GITHUB_TOKEN"], "ghp_abc123")
	}
	if result.Env["MY_API_KEY"] != "key_xyz" {
		t.Errorf("MY_API_KEY = %q, want %q", result.Env["MY_API_KEY"], "key_xyz")
	}
	if len(result.Args) != 0 {
		t.Errorf("Args = %v, want empty", result.Args)
	}
	if result.Cleanup != nil {
		t.Error("Cleanup should be nil for env injection")
	}
}

func TestApplyBindings_ArgInjection_SpaceFormat(t *testing.T) {
	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "token"},
			Inject: registry.CredentialInject{Type: "arg", Flag: "--token"},
		},
	}
	values := []string{"my-secret-token"}

	result, err := ApplyBindings("test-tool", bindings, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}

	if len(result.Args) != 2 {
		t.Fatalf("Args length = %d, want 2", len(result.Args))
	}
	if result.Args[0] != "--token" {
		t.Errorf("Args[0] = %q, want %q", result.Args[0], "--token")
	}
	if result.Args[1] != "my-secret-token" {
		t.Errorf("Args[1] = %q, want %q", result.Args[1], "my-secret-token")
	}
}

func TestApplyBindings_ArgInjection_EqFormat(t *testing.T) {
	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "token"},
			Inject: registry.CredentialInject{Type: "arg", Flag: "--token", Format: "eq"},
		},
	}
	values := []string{"my-secret-token"}

	result, err := ApplyBindings("test-tool", bindings, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}

	if len(result.Args) != 1 {
		t.Fatalf("Args length = %d, want 1", len(result.Args))
	}
	if result.Args[0] != "--token=my-secret-token" {
		t.Errorf("Args[0] = %q, want %q", result.Args[0], "--token=my-secret-token")
	}
}

func TestApplyBindings_FileInjection_Managed_JSON(t *testing.T) {
	home := setupHome(t)

	// Create a source config file that will be copied to temp
	srcDir := filepath.Join(home, "original-config")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	srcPath := filepath.Join(srcDir, "config.json")
	original := map[string]interface{}{
		"version": "1.0",
		"oauth":   map[string]interface{}{"client_id": "my-client"},
	}
	origData, _ := json.Marshal(original)
	if err := os.WriteFile(srcPath, origData, 0600); err != nil {
		t.Fatalf("failed to write original config: %v", err)
	}

	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "access_token"},
			Inject: registry.CredentialInject{
				Type:       "file",
				Path:       srcPath,
				ConfigEnv:  "TOOL_CONFIG_PATH",
				FileFormat: "json",
				Fields: map[string]string{
					"oauth.access_token": "{{value}}",
				},
			},
		},
	}
	values := []string{"tok_resolved_xyz"}

	result, err := ApplyBindings("test-tool", bindings, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}

	// config_env should be set to the ephemeral temp file path
	tmpPath, ok := result.Env["TOOL_CONFIG_PATH"]
	if !ok {
		t.Fatal("TOOL_CONFIG_PATH not set in result.Env")
	}

	// Verify the temp file exists
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse temp file JSON: %v", err)
	}

	// Original data should be preserved
	if parsed["version"] != "1.0" {
		t.Errorf("version = %v, want %q", parsed["version"], "1.0")
	}

	oauth, ok := parsed["oauth"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected oauth to be a map, got %T", parsed["oauth"])
	}
	// Original field preserved
	if oauth["client_id"] != "my-client" {
		t.Errorf("oauth.client_id = %q, want %q", oauth["client_id"], "my-client")
	}
	// New field injected
	if oauth["access_token"] != "tok_resolved_xyz" {
		t.Errorf("oauth.access_token = %q, want %q", oauth["access_token"], "tok_resolved_xyz")
	}

	// Cleanup should be non-nil
	if result.Cleanup == nil {
		t.Fatal("Cleanup should be non-nil for file injection")
	}

	// Temp file should be within ANYCLI_HOME/tmp/
	expectedPrefix := filepath.Join(home, "tmp", "test-tool")
	if !strings.HasPrefix(tmpPath, expectedPrefix) {
		t.Errorf("temp file path %q should be under %q", tmpPath, expectedPrefix)
	}
}

func TestApplyBindings_FileInject_NoConfigEnv_ReturnsError(t *testing.T) {
	setupHome(t)

	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "token"},
			Inject: registry.CredentialInject{
				Type:       "file",
				Path:       "/some/path/config.json",
				FileFormat: "json",
				// No ConfigEnv or ConfigFlag set
				Fields: map[string]string{
					"token": "{{value}}",
				},
			},
		},
	}
	values := []string{"tok_abc"}

	_, err := ApplyBindings("test-tool", bindings, values)
	if err == nil {
		t.Fatal("expected error for file inject without config_env/config_flag")
	}
	if !strings.Contains(err.Error(), "config_env") {
		t.Errorf("error message should mention config_env, got: %v", err)
	}
}

func TestApplyBindings_Cleanup_RemovesTempFiles(t *testing.T) {
	home := setupHome(t)

	// Create source file
	srcDir := filepath.Join(home, "src-config")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	srcPath := filepath.Join(srcDir, "app.json")
	if err := os.WriteFile(srcPath, []byte(`{"existing": true}`), 0600); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "token"},
			Inject: registry.CredentialInject{
				Type:       "file",
				Path:       srcPath,
				ConfigEnv:  "APP_CONFIG",
				FileFormat: "json",
				Fields: map[string]string{
					"token": "{{value}}",
				},
			},
		},
	}
	values := []string{"secret_tok"}

	result, err := ApplyBindings("cleanup-tool", bindings, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}
	if result.Cleanup == nil {
		t.Fatal("Cleanup should be non-nil")
	}

	tmpPath := result.Env["APP_CONFIG"]
	if tmpPath == "" {
		t.Fatal("APP_CONFIG env var not set")
	}

	// Verify temp file exists before cleanup
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("temp file should exist before cleanup: %v", err)
	}

	// Run cleanup
	result.Cleanup()

	// Verify temp file is removed after cleanup
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file should be removed after cleanup, stat error: %v", err)
	}
}

// Regression: yaml file injection must use the format handler (not raw
// overwrite) and must preserve non-credential settings from the original
// config. The original is copied to the ephemeral temp file and patched there;
// the original on disk is never modified.
func TestApplyBindings_FileInjection_Managed_YAML(t *testing.T) {
	home := setupHome(t)

	targetPath := filepath.Join(home, "config", "creds.yaml")

	// Create an existing YAML file with non-credential config
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	originalContent := "region: us-west-2\nformat: json\n"
	if err := os.WriteFile(targetPath, []byte(originalContent), 0600); err != nil {
		t.Fatalf("failed to write existing yaml: %v", err)
	}

	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "token"},
			Inject: registry.CredentialInject{
				Type:       "file",
				Path:       targetPath,
				ConfigEnv:  "TOOL_CONFIG",
				FileFormat: "yaml",
				Fields: map[string]string{
					"api_key": "{{.Value}}",
				},
			},
		},
	}
	values := []string{"my_secret_key"}

	result, err := ApplyBindings("test-tool", bindings, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}
	if result.Cleanup == nil {
		t.Fatal("Cleanup should be non-nil for file injection")
	}

	tmpPath := result.Env["TOOL_CONFIG"]
	if tmpPath == "" {
		t.Fatal("TOOL_CONFIG env var not set")
	}

	// Verify the temp file was patched (not overwritten)
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}
	content := string(data)

	// Must contain the new credential
	if !strings.Contains(content, "my_secret_key") {
		t.Error("patched file should contain the injected credential")
	}
	// Must still contain the original non-credential config
	if !strings.Contains(content, "region") {
		t.Error("patched temp file must preserve existing 'region' field — raw overwrite would destroy it")
	}

	// The original on-disk file must be untouched.
	orig, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read original file: %v", err)
	}
	if string(orig) != originalContent {
		t.Errorf("original file was modified: got %q, want %q", string(orig), originalContent)
	}
}

// Regression: {{.Value}} template syntax must work (not just {{value}})
func TestResolveTemplate_DotValueSyntax(t *testing.T) {
	got := resolveTemplate("prefix_{{.Value}}_suffix", "SECRET")
	if got != "prefix_SECRET_suffix" {
		t.Errorf("resolveTemplate with {{.Value}} = %q, want %q", got, "prefix_SECRET_suffix")
	}
	// Legacy syntax should also work
	got2 := resolveTemplate("prefix_{{value}}_suffix", "SECRET")
	if got2 != "prefix_SECRET_suffix" {
		t.Errorf("resolveTemplate with {{value}} = %q, want %q", got2, "prefix_SECRET_suffix")
	}
}

func TestApplyBindings_SkipsEmptyValues(t *testing.T) {
	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "token"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "MY_TOKEN"},
		},
		{
			Source: registry.CredentialSource{Field: "missing"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "MISSING_KEY"},
		},
	}
	values := []string{"real-token", ""}

	result, err := ApplyBindings("test-tool", bindings, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}

	if result.Env["MY_TOKEN"] != "real-token" {
		t.Errorf("MY_TOKEN = %q, want %q", result.Env["MY_TOKEN"], "real-token")
	}
	if _, exists := result.Env["MISSING_KEY"]; exists {
		t.Error("MISSING_KEY should not be set for empty value")
	}
}

func TestApplyBindings_LengthMismatch(t *testing.T) {
	bindings := []registry.CredentialBinding{
		{
			Source: registry.CredentialSource{Field: "token"},
			Inject: registry.CredentialInject{Type: "env", EnvVar: "TOK"},
		},
	}
	values := []string{"a", "b"} // length mismatch

	_, err := ApplyBindings("test-tool", bindings, values)
	if err == nil {
		t.Fatal("expected error for length mismatch")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error should mention mismatch, got: %v", err)
	}
}

// TestApplyBindings_BundledGitHubDefinition runs the shipped github
// definition's bindings through the inject framework end to end: the resolved
// access_token must land in the GH_TOKEN env var (design 003 toolset).
func TestApplyBindings_BundledGitHubDefinition(t *testing.T) {
	def, err := definitions.LoadBundled("github")
	if err != nil {
		t.Fatalf("LoadBundled(github) failed: %v", err)
	}
	values := valuesForBindings(def.Auth.Credentials, map[string]string{"access_token": "ghs_minted"})

	result, err := ApplyBindings("github", def.Auth.Credentials, values)
	if err != nil {
		t.Fatalf("ApplyBindings returned error: %v", err)
	}
	if result.Env["GH_TOKEN"] != "ghs_minted" {
		t.Errorf("GH_TOKEN = %q, want ghs_minted", result.Env["GH_TOKEN"])
	}
	if len(result.Args) != 0 {
		t.Errorf("Args = %v, want empty (env-only injection)", result.Args)
	}
}
