package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/shipbase/anycli/internal/registry"
)

// setupHome creates a temp ANYCLI_HOME with required subdirectories
// and sets the environment variable. It returns the home path and a
// cleanup function that restores the original env.
func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ANYCLI_HOME", home)

	dirs := []string{"registry", "bin", "credentials", "tools", "cache", "tmp"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(home, d), 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}
	return home
}

// writeDefinition writes a registry.Definition as JSON into the registry dir.
func writeDefinition(t *testing.T, home string, def *registry.Definition) {
	t.Helper()
	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal definition: %v", err)
	}
	path := filepath.Join(home, "registry", def.Name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write definition: %v", err)
	}
}

// writeCredential writes a local credential file for the given tool.
func writeCredential(t *testing.T, home, toolName string, creds map[string]string) {
	t.Helper()
	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("failed to marshal credentials: %v", err)
	}
	path := filepath.Join(home, "credentials", toolName+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write credentials: %v", err)
	}
}

// echoBinary returns the path to /bin/echo (or /usr/bin/echo on some systems).
func echoBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{"/bin/echo", "/usr/bin/echo"}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("echo binary not found; skipping test")
	return ""
}

// trueBinary returns the path to /usr/bin/true (or /bin/true).
func trueBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{"/usr/bin/true", "/bin/true"}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("true binary not found; skipping test")
	return ""
}

// falseBinary returns the path to /usr/bin/false (or /bin/false).
func falseBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{"/usr/bin/false", "/bin/false"}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("false binary not found; skipping test")
	return ""
}

func TestRun_ToolNotInstalled(t *testing.T) {
	setupHome(t)

	exitCode, err := Run("nonexistent-tool", []string{})
	if err == nil {
		t.Fatal("expected error for non-installed tool, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestRun_WithEnvCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active so we use local credentials
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	echoPath := echoBinary(t)

	def := &registry.Definition{
		Name:        "test-env-cred",
		Description: "test tool with env credential",
		Binary:      "echo",
		Resolve:     echoPath,
		Auth: &registry.AuthConfig{
			Credentials: []registry.CredentialBinding{
				{
					Source: registry.CredentialSource{
						LocalKey: "MY_TOKEN",
					},
					Inject: registry.CredentialInject{
						Type:   "env",
						EnvVar: "TEST_TOKEN",
					},
				},
			},
		},
	}
	writeDefinition(t, home, def)
	writeCredential(t, home, "test-env-cred", map[string]string{
		"MY_TOKEN": "secret-value-123",
	})

	// echo will just print args; we mainly verify it runs without error
	exitCode, err := Run("test-env-cred", []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRun_NoAuth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	truePath := trueBinary(t)

	def := &registry.Definition{
		Name:        "test-noauth",
		Description: "tool with no auth",
		Binary:      "true",
		Resolve:     truePath,
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-noauth", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRun_ServiceType(t *testing.T) {
	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	def := &registry.Definition{
		Name:        "test-service",
		Type:        "service",
		Description: "service type tool",
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-service", []string{})
	if err == nil {
		t.Fatal("expected error for unregistered service, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	// Verify the error mentions the service not being registered
	if err.Error() != `no built-in service registered for "test-service"` {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRun_ResolveBinary_AbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	truePath := trueBinary(t)

	def := &registry.Definition{
		Name:        "test-abs-resolve",
		Description: "tool with absolute resolve path",
		Binary:      "true",
		Resolve:     truePath,
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-abs-resolve", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRun_ResolveBinary_AbsolutePath_NotFound(t *testing.T) {
	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	def := &registry.Definition{
		Name:        "test-abs-missing",
		Description: "tool with missing absolute path",
		Binary:      "nonexistent",
		Resolve:     "/nonexistent/path/to/binary",
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-abs-missing", []string{})
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestRun_ResolveBinary_Which(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	// Create a temporary directory with a fake binary and put it on PATH
	binDir := filepath.Join(t.TempDir(), "testbin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a simple executable script
	scriptPath := filepath.Join(binDir, "test-which-tool")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	// Prepend our test bin dir to PATH (after the shim dir which will be skipped)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	def := &registry.Definition{
		Name:        "test-which-tool",
		Description: "tool resolved via PATH",
		Binary:      "test-which-tool",
		Resolve:     "", // empty means search PATH
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-which-tool", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRun_ResolveBinary_Which_NotFound(t *testing.T) {
	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	// Set PATH to empty so nothing is found, except the shim dir
	t.Setenv("PATH", filepath.Join(home, "bin"))

	def := &registry.Definition{
		Name:        "test-not-in-path",
		Description: "tool not found in PATH",
		Binary:      "definitely-not-a-real-binary",
		Resolve:     "",
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-not-in-path", []string{})
	if err == nil {
		t.Fatal("expected error for binary not in PATH, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestRun_NonZeroExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	falsePath := falseBinary(t)

	def := &registry.Definition{
		Name:        "test-false",
		Description: "tool that exits non-zero",
		Binary:      "false",
		Resolve:     falsePath,
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-false", []string{})
	// false returns exit code 1, which causes exec error
	if exitCode == 0 {
		t.Error("expected non-zero exit code from false binary")
	}
	// err may or may not be set depending on how the system handles non-zero exits
	_ = err
}

func TestRun_WithBeforeHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	echoPath := echoBinary(t)

	def := &registry.Definition{
		Name:        "test-before-hook",
		Description: "tool with before hook",
		Binary:      "echo",
		Resolve:     echoPath,
		Before: []registry.Rule{
			{
				Name: "append-json",
				Rule: "append_flag",
				Config: map[string]interface{}{
					"flag": "--json",
				},
			},
		},
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-before-hook", []string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRun_WithAfterHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	echoPath := echoBinary(t)

	def := &registry.Definition{
		Name:        "test-after-hook",
		Description: "tool with after hook",
		Binary:      "echo",
		Resolve:     echoPath,
		After: []registry.Rule{
			{
				Name: "ensure-json",
				Rule: "ensure_json",
			},
		},
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-after-hook", []string{"hello world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestResolveBinary_AbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	truePath := trueBinary(t)

	def := &registry.Definition{
		Name:    "test",
		Binary:  "true",
		Resolve: truePath,
	}

	got, err := resolveBinary(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != truePath {
		t.Errorf("expected %s, got %s", truePath, got)
	}
}

func TestResolveBinary_AbsolutePath_Missing(t *testing.T) {
	def := &registry.Definition{
		Name:    "test",
		Binary:  "missing",
		Resolve: "/nonexistent/path/to/binary",
	}

	_, err := resolveBinary(def)
	if err == nil {
		t.Fatal("expected error for missing absolute path")
	}
}

func TestResolveBinary_Which(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := t.TempDir()
	t.Setenv("ANYCLI_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a temp directory with a binary
	binDir := filepath.Join(t.TempDir(), "testbin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	fakeBin := filepath.Join(binDir, "my-tool")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Join(home, "bin"))

	def := &registry.Definition{
		Name:    "my-tool",
		Binary:  "my-tool",
		Resolve: "",
	}

	got, err := resolveBinary(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeBin {
		t.Errorf("expected %s, got %s", fakeBin, got)
	}
}

func TestResolveBinary_SkipsShimDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := t.TempDir()
	t.Setenv("ANYCLI_HOME", home)
	shimDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(shimDir, 0755); err != nil {
		t.Fatalf("failed to create shim dir: %v", err)
	}

	// Put a binary in the shim dir (should be skipped)
	shimBin := filepath.Join(shimDir, "my-tool")
	if err := os.WriteFile(shimBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write shim binary: %v", err)
	}

	// Put the same binary in a real dir (should be found)
	realDir := filepath.Join(t.TempDir(), "realbin")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}
	realBin := filepath.Join(realDir, "my-tool")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write real binary: %v", err)
	}

	// PATH: shim dir first, then real dir
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+realDir)

	def := &registry.Definition{
		Name:    "my-tool",
		Binary:  "my-tool",
		Resolve: "",
	}

	got, err := resolveBinary(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != realBin {
		t.Errorf("expected %s (real bin), got %s", realBin, got)
	}
}

func TestResolveBinary_WhichResolveValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := t.TempDir()
	t.Setenv("ANYCLI_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	binDir := filepath.Join(t.TempDir(), "testbin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	fakeBin := filepath.Join(binDir, "my-tool")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	t.Setenv("PATH", binDir)

	// Resolve = "which" should behave the same as Resolve = ""
	def := &registry.Definition{
		Name:    "my-tool",
		Binary:  "my-tool",
		Resolve: "which",
	}

	got, err := resolveBinary(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeBin {
		t.Errorf("expected %s, got %s", fakeBin, got)
	}
}

func TestBuildEnv(t *testing.T) {
	env := map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}

	result := buildEnv(env)

	// Should contain all original env vars plus our additions
	found := make(map[string]bool)
	for _, e := range result {
		if e == "FOO=bar" {
			found["FOO"] = true
		}
		if e == "BAZ=qux" {
			found["BAZ"] = true
		}
	}

	if !found["FOO"] {
		t.Error("expected FOO=bar in environment")
	}
	if !found["BAZ"] {
		t.Error("expected BAZ=qux in environment")
	}
}

func TestUniqueVaultTools(t *testing.T) {
	bindings := []registry.CredentialBinding{
		{Source: registry.CredentialSource{VaultTool: "github"}},
		{Source: registry.CredentialSource{VaultTool: "github"}},
		{Source: registry.CredentialSource{VaultTool: "notion"}},
		{Source: registry.CredentialSource{VaultTool: ""}},
		{Source: registry.CredentialSource{VaultTool: "github"}},
	}

	result := uniqueVaultTools(bindings)
	if len(result) != 2 {
		t.Fatalf("expected 2 unique vault tools, got %d: %v", len(result), result)
	}

	expected := map[string]bool{"github": true, "notion": true}
	for _, vt := range result {
		if !expected[vt] {
			t.Errorf("unexpected vault tool: %s", vt)
		}
	}
}

func TestUniqueVaultTools_Empty(t *testing.T) {
	result := uniqueVaultTools(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}

	result = uniqueVaultTools([]registry.CredentialBinding{
		{Source: registry.CredentialSource{LocalKey: "token"}},
	})
	if len(result) != 0 {
		t.Errorf("expected empty result for bindings with no vault_tool, got %v", result)
	}
}

func TestRun_AuthWithCredentials_ResolvesCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	truePath := trueBinary(t)

	// Definition with Auth that has credentials; credential resolution runs.
	def := &registry.Definition{
		Name:        "test-creds",
		Description: "tool with credential-based auth",
		Binary:      "true",
		Resolve:     truePath,
		Auth: &registry.AuthConfig{
			Credentials: []registry.CredentialBinding{
				{
					Source: registry.CredentialSource{LocalKey: "TOKEN"},
					Inject: registry.CredentialInject{Type: "env", EnvVar: "MY_TOKEN"},
				},
			},
		},
	}
	writeDefinition(t, home, def)
	writeCredential(t, home, "test-creds", map[string]string{
		"TOKEN": "secret-value",
	})

	exitCode, err := Run("test-creds", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRun_EmptyCredentials_NoError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	home := setupHome(t)

	// Ensure vault mode is NOT active
	t.Setenv("ANYCLI_VAULT_URL", "")
	t.Setenv("ANYCLI_VAULT_TOKEN", "")
	t.Setenv("ANYCLI_VAULT_WORKSPACE_ID", "")

	truePath := trueBinary(t)

	// Auth config present but with empty credentials slice
	def := &registry.Definition{
		Name:        "test-empty-creds",
		Description: "tool with empty credentials list",
		Binary:      "true",
		Resolve:     truePath,
		Auth: &registry.AuthConfig{
			Credentials: []registry.CredentialBinding{},
		},
	}
	writeDefinition(t, home, def)

	exitCode, err := Run("test-empty-creds", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}
