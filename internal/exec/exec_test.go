package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/credential"
	"github.com/heliohq/anycli/internal/exec/binresolve"
	"github.com/heliohq/anycli/internal/registry"
	"github.com/heliohq/anycli/internal/tools"
	"github.com/spf13/cobra"
)

// setupHome creates a temp ANYCLI_HOME and points the env at it.
func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ANYCLI_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	return home
}

// useDefinitions installs a synthetic definition loader for the duration of a
// test, restoring the embedded loader afterward.
func useDefinitions(t *testing.T, defs map[string]*registry.Definition) {
	t.Helper()
	orig := loadDefinition
	loadDefinition = func(name string) (*registry.Definition, error) {
		if d, ok := defs[name]; ok {
			return d, nil
		}
		return nil, fmt.Errorf("no bundled definition for %q", name)
	}
	t.Cleanup(func() { loadDefinition = orig })
}

// newTestEngine builds an Engine backed by a fresh in-memory cache and returns
// both so a test can inspect the cache after Execute.
func newTestEngine(t *testing.T) (*Engine, credential.Cache) {
	t.Helper()
	cache := credential.NewMemoryCache()
	e, err := NewEngine(cache)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	return e, cache
}

// fixedResolver returns the same credential data for every tool.
type fixedResolver struct {
	data map[string]string
}

func (r fixedResolver) Resolve(ctx context.Context, tool credential.Tool, account string) (*credential.Credential, error) {
	return &credential.Credential{Data: r.data, CacheUntil: time.Now().Add(time.Hour)}, nil
}

// nilResolver always returns no credential.
type nilResolver struct{}

func (nilResolver) Resolve(ctx context.Context, tool credential.Tool, account string) (*credential.Credential, error) {
	return nil, nil
}

type fixedService struct {
	result tools.ExecutionResult
	err    error
}

func (s fixedService) Execute(context.Context, []string, map[string]string) (tools.ExecutionResult, error) {
	return s.result, s.err
}

func (s fixedService) NewCommandTree() *cobra.Command {
	return &cobra.Command{Use: "fixed"}
}

func echoBinary(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"/bin/echo", "/usr/bin/echo"} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("echo binary not found; skipping test")
	return ""
}

func trueBinary(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"/usr/bin/true", "/bin/true"} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("true binary not found; skipping test")
	return ""
}

func falseBinary(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"/usr/bin/false", "/bin/false"} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("false binary not found; skipping test")
	return ""
}

func TestNewEngine_NilCache(t *testing.T) {
	if _, err := NewEngine(nil); err == nil {
		t.Fatal("expected error for nil cache")
	}
}

func TestExecute_NilResolver(t *testing.T) {
	setupHome(t)
	e, _ := newTestEngine(t)
	exitCode, err := e.Execute(context.Background(), "gh", nil, nil, "")
	if err == nil {
		t.Fatal("expected error for nil resolver")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	setupHome(t)
	useDefinitions(t, map[string]*registry.Definition{})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "nonexistent-tool", []string{}, nilResolver{}, "")
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestExecute_WithEnvCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	echoPath := echoBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-env-cred": {
			Name:    "test-env-cred",
			Binary:  "echo",
			Resolve: echoPath,
			Auth: &registry.AuthConfig{
				Credentials: []registry.CredentialBinding{
					{
						Source: registry.CredentialSource{Field: "access_token"},
						Inject: registry.CredentialInject{Type: "env", EnvVar: "TEST_TOKEN"},
					},
				},
			},
		},
	})
	resolver := fixedResolver{data: map[string]string{"access_token": "secret-value-123"}}
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-env-cred", []string{"hello"}, resolver, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestExecute_NoAuth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	truePath := trueBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-noauth": {Name: "test-noauth", Binary: "true", Resolve: truePath},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-noauth", []string{}, nilResolver{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestExecute_ServiceType_Unregistered(t *testing.T) {
	setupHome(t)
	useDefinitions(t, map[string]*registry.Definition{
		"test-service": {Name: "test-service", Type: "service"},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-service", []string{}, nilResolver{}, "")
	if err == nil {
		t.Fatal("expected error for unregistered service, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if err.Error() != `no built-in service registered for "test-service"` {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExecute_ServiceMarksOnlyRejectedCredentialsStale(t *testing.T) {
	cases := []struct {
		name               string
		toolName           string
		credentialRejected bool
		wantStale          bool
	}{
		{name: "ordinary command failure stays fresh", toolName: "test-service-command-failure", wantStale: false},
		{name: "credential rejection becomes stale", toolName: "test-service-credential-rejected", credentialRejected: true, wantStale: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useDefinitions(t, map[string]*registry.Definition{
				tc.toolName: {
					Name: tc.toolName,
					Type: "service",
					Auth: &registry.AuthConfig{Credentials: []registry.CredentialBinding{
						{
							Source: registry.CredentialSource{Field: "access_token"},
							Inject: registry.CredentialInject{Type: "env", EnvVar: "TEST_TOKEN"},
						},
					}},
				},
			})
			tools.RegisterService(tc.toolName, fixedService{result: tools.ExecutionResult{
				ExitCode:           1,
				CredentialRejected: tc.credentialRejected,
			}})

			engine, cache := newTestEngine(t)
			resolver := fixedResolver{data: map[string]string{"access_token": "test-token"}}
			exitCode, err := engine.Execute(context.Background(), tc.toolName, nil, resolver, "account-1")
			if err != nil {
				t.Fatalf("Execute returned unexpected error: %v", err)
			}
			if exitCode != 1 {
				t.Fatalf("exit code = %d, want 1", exitCode)
			}

			entry, ok := cache.Get(credential.CacheKey(tc.toolName, "account-1"))
			if !ok {
				t.Fatal("credential was not cached")
			}
			if entry.Stale != tc.wantStale {
				t.Errorf("credential stale = %t, want %t", entry.Stale, tc.wantStale)
			}
		})
	}
}

func TestExecute_ResolveBinary_AbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	truePath := trueBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-abs-resolve": {Name: "test-abs-resolve", Binary: "true", Resolve: truePath},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-abs-resolve", []string{}, nilResolver{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestExecute_ResolveBinary_AbsolutePath_NotFound(t *testing.T) {
	setupHome(t)
	useDefinitions(t, map[string]*registry.Definition{
		"test-abs-missing": {Name: "test-abs-missing", Binary: "nonexistent", Resolve: "/nonexistent/path/to/binary"},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-abs-missing", []string{}, nilResolver{}, "")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestExecute_ResolveBinary_Which(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	home := setupHome(t)

	binDir := filepath.Join(t.TempDir(), "testbin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	scriptPath := filepath.Join(binDir, "test-which-tool")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Join(home, "bin"))

	useDefinitions(t, map[string]*registry.Definition{
		"test-which-tool": {Name: "test-which-tool", Binary: "test-which-tool", Resolve: ""},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-which-tool", []string{}, nilResolver{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestExecute_ResolveBinary_Which_NotFound(t *testing.T) {
	home := setupHome(t)
	t.Setenv("PATH", filepath.Join(home, "bin"))

	useDefinitions(t, map[string]*registry.Definition{
		"test-not-in-path": {Name: "test-not-in-path", Binary: "definitely-not-a-real-binary", Resolve: ""},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-not-in-path", []string{}, nilResolver{}, "")
	if err == nil {
		t.Fatal("expected error for binary not in PATH, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestExecute_NonZeroExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	falsePath := falseBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-false": {Name: "test-false", Binary: "false", Resolve: falsePath},
	})
	e, _ := newTestEngine(t)

	exitCode, _ := e.Execute(context.Background(), "test-false", []string{}, nilResolver{}, "")
	if exitCode == 0 {
		t.Error("expected non-zero exit code from false binary")
	}
}

func TestExecute_StaleMarkOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	falsePath := falseBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-fail-creds": {
			Name:    "test-fail-creds",
			Binary:  "false",
			Resolve: falsePath,
			Auth: &registry.AuthConfig{
				Credentials: []registry.CredentialBinding{
					{
						Source: registry.CredentialSource{Field: "access_token"},
						Inject: registry.CredentialInject{Type: "env", EnvVar: "TOK"},
					},
				},
			},
		},
	})
	resolver := fixedResolver{data: map[string]string{"access_token": "tok"}}
	e, cache := newTestEngine(t)

	exitCode, _ := e.Execute(context.Background(), "test-fail-creds", []string{}, resolver, "")
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}

	// After a failure, the cached credential should be marked stale.
	cached, ok := cache.Get(credential.CacheKey("test-fail-creds", ""))
	if !ok || cached == nil {
		t.Fatal("expected a cache entry to exist")
	}
	if !cached.Stale {
		t.Error("expected cache to be marked stale after non-zero exit")
	}
}

func TestExecute_WithBeforeHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	echoPath := echoBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-before-hook": {
			Name:    "test-before-hook",
			Binary:  "echo",
			Resolve: echoPath,
			Before: []registry.Rule{
				{Name: "append-json", Rule: "append_flag", Config: map[string]interface{}{"flag": "--json"}},
			},
		},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-before-hook", []string{"test"}, nilResolver{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestExecute_WithAfterHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	echoPath := echoBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-after-hook": {
			Name:    "test-after-hook",
			Binary:  "echo",
			Resolve: echoPath,
			After: []registry.Rule{
				{Name: "ensure-json", Rule: "ensure_json"},
			},
		},
	})
	e, _ := newTestEngine(t)

	exitCode, err := e.Execute(context.Background(), "test-after-hook", []string{"hello world"}, nilResolver{}, "")
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
	def := &registry.Definition{Name: "test", Binary: "true", Resolve: truePath}

	got, err := ResolveBinary(context.Background(), def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != truePath {
		t.Errorf("expected %s, got %s", truePath, got)
	}
}

func TestResolveBinary_AbsolutePath_Missing(t *testing.T) {
	def := &registry.Definition{Name: "test", Binary: "missing", Resolve: "/nonexistent/path/to/binary"}
	if _, err := ResolveBinary(context.Background(), def); err == nil {
		t.Fatal("expected error for missing absolute path")
	}
}

func TestResolveBinary_Which(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	home := setupHome(t)

	binDir := filepath.Join(t.TempDir(), "testbin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	fakeBin := filepath.Join(binDir, "my-tool")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Join(home, "bin"))

	def := &registry.Definition{Name: "my-tool", Binary: "my-tool", Resolve: ""}
	got, err := ResolveBinary(context.Background(), def)
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
	home := setupHome(t)
	shimDir := filepath.Join(home, "bin")

	shimBin := filepath.Join(shimDir, "my-tool")
	if err := os.WriteFile(shimBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write shim binary: %v", err)
	}
	realDir := filepath.Join(t.TempDir(), "realbin")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}
	realBin := filepath.Join(realDir, "my-tool")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write real binary: %v", err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+realDir)

	def := &registry.Definition{Name: "my-tool", Binary: "my-tool", Resolve: ""}
	got, err := ResolveBinary(context.Background(), def)
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
	setupHome(t)

	binDir := filepath.Join(t.TempDir(), "testbin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	fakeBin := filepath.Join(binDir, "my-tool")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}
	t.Setenv("PATH", binDir)

	def := &registry.Definition{Name: "my-tool", Binary: "my-tool", Resolve: "which"}
	got, err := ResolveBinary(context.Background(), def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeBin {
		t.Errorf("expected %s, got %s", fakeBin, got)
	}
}

func TestBuildEnv(t *testing.T) {
	env := map[string]string{"FOO": "bar", "BAZ": "qux"}
	result := buildEnv(env)

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

func TestExecute_StaleMarkHitsOnlyFailingAccount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	falsePath := falseBinary(t)

	useDefinitions(t, map[string]*registry.Definition{
		"test-multi-acct": {
			Name:    "test-multi-acct",
			Binary:  "false",
			Resolve: falsePath,
			Auth: &registry.AuthConfig{
				Credentials: []registry.CredentialBinding{
					{
						Source: registry.CredentialSource{Field: "access_token"},
						Inject: registry.CredentialInject{Type: "env", EnvVar: "TOK"},
					},
				},
			},
		},
	})
	e, cache := newTestEngine(t)

	// Pre-seed fresh entries for two accounts of the same tool.
	for _, account := range []string{"a1", "a2"} {
		cache.Set(credential.CacheKey("test-multi-acct", account), &credential.CacheEntry{
			FetchedAt:  time.Now(),
			CacheUntil: time.Now().Add(time.Hour),
			Fields:     map[string]string{"access_token": "tok-" + account},
		})
	}

	exitCode, _ := e.Execute(context.Background(), "test-multi-acct", []string{}, fixedResolver{}, "a1")
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}

	a1, ok := cache.Get(credential.CacheKey("test-multi-acct", "a1"))
	if !ok || !a1.Stale {
		t.Error("expected the failing account's entry (a1) to be marked stale")
	}
	a2, ok := cache.Get(credential.CacheKey("test-multi-acct", "a2"))
	if !ok || a2.Stale {
		t.Error("the other account's entry (a2) must NOT be marked stale")
	}
}

// TestResolveBinary_GitHubDefinitionPinThenPATH pins the real gh definition's
// resolution order now that it carries a direct-download source: a PATH hit
// (level ②) resolves without engaging lazy install, and a pinned gh (level ①)
// wins over the same PATH entry. The lazy-install miss path itself downloads
// from github.com and is covered by the env-guarded TestE2ERealGhLazyInstall
// in binresolve, not here.
func TestResolveBinary_GitHubDefinitionPinThenPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	setupHome(t)
	pinRoot := t.TempDir()
	t.Setenv("HELIO_BIN_DIR", pinRoot)

	def, err := definitions.LoadBundled("github")
	if err != nil {
		t.Fatalf("LoadBundled(github) failed: %v", err)
	}

	// PATH hit with an empty pin root: resolves to the PATH entry.
	binDir := t.TempDir()
	fakeGh := filepath.Join(binDir, "gh")
	if err := os.WriteFile(fakeGh, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", binDir)
	got, err := ResolveBinary(context.Background(), def)
	if err != nil {
		t.Fatalf("ResolveBinary: %v", err)
	}
	if got != fakeGh {
		t.Errorf("ResolveBinary = %q, want PATH hit %q", got, fakeGh)
	}

	// Pinned gh present: level ① wins over the same PATH entry.
	pinned := filepath.Join(pinRoot, "versions", "github", def.Source.Version, binresolve.Platform(def.Source), "gh")
	if err := os.MkdirAll(filepath.Dir(pinned), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinned, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write pinned gh: %v", err)
	}
	got, err = ResolveBinary(context.Background(), def)
	if err != nil {
		t.Fatalf("ResolveBinary with pin: %v", err)
	}
	if got != pinned {
		t.Errorf("ResolveBinary = %q, want pinned %q", got, pinned)
	}
}
