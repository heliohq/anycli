package definitions

import (
	"strings"
	"testing"
)

// TestLoadBundled_NotFound asserts the embedded-definitions mechanism compiles
// and degrades gracefully with zero shipped definitions: any lookup returns the
// not-found error rather than panicking or failing to build. When real
// definitions are added under tools/, add load tests for each.
func TestLoadBundled_NotFound(t *testing.T) {
	_, err := LoadBundled("definitely-not-a-shipped-tool")
	if err == nil {
		t.Fatal("expected an error for an unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "no bundled definition") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestLoadBundled_ShippedDefinitions asserts every shipped definition loads
// and exposes a complete credential-injection shape. gate-probe is the one
// pinned exception (design 318 §E2E Testing Harness): the approval-gate probe
// is credential-free by contract, so it must ship with NO auth block at all.
func TestLoadBundled_ShippedDefinitions(t *testing.T) {
	bundled, err := ListBundled()
	if err != nil {
		t.Fatalf("ListBundled failed: %v", err)
	}
	if len(bundled) == 0 {
		t.Fatal("no bundled tool definitions")
	}
	for _, def := range bundled {
		t.Run(def.Name, func(t *testing.T) {
			if def.Description == "" {
				t.Error("Description is empty")
			}
			if def.Name == "gate-probe" {
				if def.Auth != nil {
					t.Fatalf("gate-probe declares an auth block %+v; design 318 pins it credential-free", def.Auth)
				}
				return
			}
			if def.Auth == nil || len(def.Auth.Credentials) == 0 {
				t.Fatal("tool has no credential bindings")
			}
			for i, binding := range def.Auth.Credentials {
				if binding.Source.Field == "" {
					t.Errorf("binding %d has no source field", i)
				}
				if binding.Inject.Type == "" {
					t.Errorf("binding %d has no injection type", i)
				}
			}
		})
	}
}

func TestLoadBundled_XCredentialBindings(t *testing.T) {
	def, err := LoadBundled("x")
	if err != nil {
		t.Fatalf("LoadBundled(x) failed: %v", err)
	}
	want := []struct {
		field  string
		envVar string
	}{
		{field: "access_token", envVar: "X_ACCESS_TOKEN"},
		{field: "user_id", envVar: "X_USER_ID"},
	}
	if def.Auth == nil || len(def.Auth.Credentials) != len(want) {
		t.Fatalf("credentials = %+v, want %d bindings", def.Auth, len(want))
	}
	for i, binding := range def.Auth.Credentials {
		if binding.Source.Field != want[i].field {
			t.Errorf("binding %d field = %q, want %q", i, binding.Source.Field, want[i].field)
		}
		if binding.Inject.Type != "env" || binding.Inject.EnvVar != want[i].envVar {
			t.Errorf("binding %d inject = %+v, want env %s", i, binding.Inject, want[i].envVar)
		}
	}
}

func TestLoadBundled_BitlyCredentialBinding(t *testing.T) {
	def, err := LoadBundled("bitly")
	if err != nil {
		t.Fatalf("LoadBundled(bitly) failed: %v", err)
	}
	if def.Type != "service" {
		t.Errorf("Type = %q, want service", def.Type)
	}
	if def.Auth == nil || len(def.Auth.Credentials) != 1 {
		t.Fatalf("credentials = %+v, want one binding", def.Auth)
	}
	binding := def.Auth.Credentials[0]
	if binding.Source.Field != "access_token" {
		t.Errorf("field = %q, want access_token", binding.Source.Field)
	}
	if binding.Inject.Type != "env" || binding.Inject.EnvVar != "BITLY_ACCESS_TOKEN" {
		t.Errorf("inject = %+v, want env BITLY_ACCESS_TOKEN", binding.Inject)
	}
}

func TestLoadBundled_FigmaCredentialBinding(t *testing.T) {
	def, err := LoadBundled("figma")
	if err != nil {
		t.Fatalf("LoadBundled(figma) failed: %v", err)
	}
	if def.Type != "service" {
		t.Errorf("Type = %q, want service", def.Type)
	}
	if def.Auth == nil || len(def.Auth.Credentials) != 1 {
		t.Fatalf("credentials = %+v, want one binding", def.Auth)
	}
	binding := def.Auth.Credentials[0]
	if binding.Source.Field != "access_token" {
		t.Errorf("field = %q, want access_token", binding.Source.Field)
	}
	if binding.Inject.Type != "env" || binding.Inject.EnvVar != "FIGMA_ACCESS_TOKEN" {
		t.Errorf("inject = %+v, want env FIGMA_ACCESS_TOKEN", binding.Inject)
	}
}

// TestLoadBundled_GateProbeShape pins the gate-probe harness definition
// (design 318 §E2E Testing Harness): service type so execTool's LoadBundled
// precondition passes, and no auth block — execution needs no credentials and
// the engine must never call the resolver for it.
func TestLoadBundled_GateProbeShape(t *testing.T) {
	def, err := LoadBundled("gate-probe")
	if err != nil {
		t.Fatalf("LoadBundled(gate-probe) failed: %v", err)
	}
	if def.Type != "service" {
		t.Errorf("Type = %q, want service", def.Type)
	}
	if def.Auth != nil {
		t.Errorf("Auth = %+v, want nil (credential-free by contract)", def.Auth)
	}
	if def.Binary != "" {
		t.Errorf("Binary = %q, want empty for a service tool", def.Binary)
	}
}

// TestLoadBundled_LarkCliShape pins the lark definition's cli-type shape: it
// wraps the official larksuite/cli binary, injects the host-minted tenant
// access token (never the app secret), and pins the bot identity via static
// env. The field names are a wire contract with the host's token projection —
// a drifted name means no injection, and the CLI then fails as not-logged-in
// instead of naming the missing field, misattributing the drift.
func TestLoadBundled_LarkCliShape(t *testing.T) {
	def, err := LoadBundled("lark")
	if err != nil {
		t.Fatalf("LoadBundled(lark) failed: %v", err)
	}
	if def.Type != "" {
		t.Errorf("Type = %q, want \"\" (cli default)", def.Type)
	}
	if def.Binary != "lark-cli" {
		t.Errorf("Binary = %q, want lark-cli", def.Binary)
	}
	if def.Source == nil || def.Source.Type != "github-release" || def.Source.Repo != "larksuite/cli" {
		t.Errorf("Source = %+v, want github-release larksuite/cli", def.Source)
	}
	want := []struct {
		field  string
		envVar string
	}{
		{field: "app_id", envVar: "LARKSUITE_CLI_APP_ID"},
		{field: "access_token", envVar: "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"},
		{field: "brand", envVar: "LARKSUITE_CLI_BRAND"},
	}
	if def.Auth == nil || len(def.Auth.Credentials) != len(want) {
		t.Fatalf("credentials = %+v, want %d bindings", def.Auth, len(want))
	}
	for i, binding := range def.Auth.Credentials {
		if binding.Source.Field != want[i].field {
			t.Errorf("binding %d field = %q, want %q", i, binding.Source.Field, want[i].field)
		}
		if binding.Inject.Type != "env" || binding.Inject.EnvVar != want[i].envVar {
			t.Errorf("binding %d inject = %+v, want env %s", i, binding.Inject, want[i].envVar)
		}
	}
	// Dual identity is deliberate: the CLI defaults to the injected bot
	// (tenant) identity, and the agent may run `lark-cli auth login`
	// (OAuth device flow) to add a user identity for `--as user` reads.
	// Strict mode would lock out that user leg — pin its ABSENCE so a
	// bot-only lock can't silently return.
	for _, r := range def.Before {
		if r.Rule != "set_env" {
			continue
		}
		envVar, _ := r.Config["env_var"].(string)
		if envVar == "LARKSUITE_CLI_STRICT_MODE" {
			t.Errorf("before rules set LARKSUITE_CLI_STRICT_MODE — dual identity (bot default + device-flow user login) must stay open")
		}
	}
}

// TestLoadBundled_GitHubCliShape pins the github definition's cli-type shape:
// it wraps the gh binary from a pinned official direct-download source (lazy
// install with mandatory per-platform sha256) and injects the minted token as
// GH_TOKEN. gh's windows zip lays bin/gh.exe at the archive root (no versioned
// top dir), so the definition must carry the binary_path_map override.
func TestLoadBundled_GitHubCliShape(t *testing.T) {
	def, err := LoadBundled("github")
	if err != nil {
		t.Fatalf("LoadBundled(github) failed: %v", err)
	}
	if def.Type != "" {
		t.Errorf("Type = %q, want \"\" (cli default)", def.Type)
	}
	if def.Binary != "gh" {
		t.Errorf("Binary = %q, want gh", def.Binary)
	}
	src := def.Source
	if src == nil {
		t.Fatal("Source missing — the gh lazy-install source must be declared")
	}
	if src.Type != "direct" {
		t.Errorf("Source.Type = %q, want direct", src.Type)
	}
	if src.Version != "2.96.0" {
		t.Errorf("Source.Version = %q, want pinned 2.96.0", src.Version)
	}
	if src.URLTemplate == "" || src.BinaryPath == "" {
		t.Errorf("Source url_template/binary_path missing: %+v", src)
	}
	if src.BinaryPathMap["windows"] == "" {
		t.Error("binary_path_map lacks the windows override — gh's windows zip has no versioned top dir")
	}
	for _, platform := range []string{"macOS-arm64", "macOS-amd64", "linux-arm64", "linux-amd64", "windows-amd64"} {
		digest, ok := src.SHA256[platform]
		if !ok {
			t.Errorf("sha256 missing for platform %s", platform)
			continue
		}
		if len(digest) != 64 {
			t.Errorf("sha256[%s] = %q, want a 64-hex digest", platform, digest)
		}
	}
	b := def.Auth.Credentials[0]
	if b.Source.Field != "access_token" {
		t.Errorf("field = %q, want access_token", b.Source.Field)
	}
	if b.Inject.Type != "env" || b.Inject.EnvVar != "GH_TOKEN" {
		t.Errorf("inject = %+v, want env GH_TOKEN", b.Inject)
	}
}

// TestLoadBundled_MongoDBShape pins the mongodb definition's mongosh-wrapper
// shape: a service-type tool whose underlying binary is the official mongosh
// with a pinned direct-download source (mandatory per-platform sha256), and
// the unchanged connection-string env binding (the provider.yaml wire
// contract — a drifted env var name means no injection).
func TestLoadBundled_MongoDBShape(t *testing.T) {
	def, err := LoadBundled("mongodb")
	if err != nil {
		t.Fatalf("LoadBundled(mongodb) failed: %v", err)
	}
	if def.Type != "service" {
		t.Errorf("Type = %q, want service", def.Type)
	}
	if def.Binary != "mongosh" {
		t.Errorf("Binary = %q, want mongosh", def.Binary)
	}
	src := def.Source
	if src == nil {
		t.Fatal("Source missing — the mongosh lazy-install source must be declared")
	}
	if src.Type != "direct" {
		t.Errorf("Source.Type = %q, want direct", src.Type)
	}
	if src.Version != "2.9.2" {
		t.Errorf("Source.Version = %q, want pinned 2.9.2", src.Version)
	}
	if src.URLTemplate == "" || src.BinaryPath == "" {
		t.Errorf("Source url_template/binary_path missing: %+v", src)
	}
	for _, platform := range []string{"darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "win32-x64"} {
		digest, ok := src.SHA256[platform]
		if !ok {
			t.Errorf("sha256 missing for platform %s", platform)
			continue
		}
		if len(digest) != 64 {
			t.Errorf("sha256[%s] = %q, want a 64-hex digest", platform, digest)
		}
	}
	if def.Auth == nil || len(def.Auth.Credentials) != 1 {
		t.Fatalf("credentials = %+v, want one binding", def.Auth)
	}
	binding := def.Auth.Credentials[0]
	if binding.Source.Field != "connection_string" {
		t.Errorf("field = %q, want connection_string", binding.Source.Field)
	}
	if binding.Inject.Type != "env" || binding.Inject.EnvVar != "MONGODB_CONNECTION_STRING" {
		t.Errorf("inject = %+v, want env MONGODB_CONNECTION_STRING", binding.Inject)
	}
}

// TestLoadBundled_DirectSourcesAreComplete validates every direct-download
// source shipped in the definitions: lazy install requires a url template, a
// pinned version, an archive binary path, and a non-empty sha256 table.
func TestLoadBundled_DirectSourcesAreComplete(t *testing.T) {
	bundled, err := ListBundled()
	if err != nil {
		t.Fatalf("ListBundled failed: %v", err)
	}
	for _, def := range bundled {
		if def.Source == nil || def.Source.Type != "direct" {
			continue
		}
		src := def.Source
		if src.URLTemplate == "" {
			t.Errorf("%s: direct source has no url_template", def.Name)
		}
		if src.Version == "" {
			t.Errorf("%s: direct source has no pinned version", def.Name)
		}
		if src.BinaryPath == "" {
			t.Errorf("%s: direct source has no binary_path", def.Name)
		}
		if len(src.SHA256) == 0 {
			t.Errorf("%s: direct source has no sha256 table — lazy install would have nothing to verify", def.Name)
		}
		for platform, digest := range src.SHA256 {
			if len(digest) != 64 {
				t.Errorf("%s: sha256[%s] = %q, want a 64-hex digest", def.Name, platform, digest)
			}
		}

		// Every sha256 platform key must be reachable by URL expansion: some
		// Go OS/arch must map (via os_map/arch_map, defaulting to identity)
		// onto the key, and ext_map must cover that Go OS when the template
		// uses {ext}. Otherwise the pinned digest is dead weight and install
		// on that platform fails only at runtime with an empty {ext} URL.
		goosSet := []string{"darwin", "linux", "windows"}
		goarchSet := []string{"amd64", "arm64"}
		mapOS := func(goos string) string {
			if m, ok := src.OsMap[goos]; ok {
				return m
			}
			return goos
		}
		mapArch := func(goarch string) string {
			if m, ok := src.ArchMap[goarch]; ok {
				return m
			}
			return goarch
		}
		for platform := range src.SHA256 {
			osName, arch, ok := strings.Cut(platform, "-")
			if !ok {
				t.Errorf("%s: sha256 platform key %q is not <os>-<arch>", def.Name, platform)
				continue
			}
			var matchedGoos []string
			for _, goos := range goosSet {
				if mapOS(goos) == osName {
					matchedGoos = append(matchedGoos, goos)
				}
			}
			if len(matchedGoos) == 0 {
				t.Errorf("%s: sha256 pins %q but no Go OS maps to %q via os_map", def.Name, platform, osName)
			}
			archMatched := false
			for _, goarch := range goarchSet {
				if mapArch(goarch) == arch {
					archMatched = true
					break
				}
			}
			if !archMatched {
				t.Errorf("%s: sha256 pins %q but no Go arch maps to %q via arch_map", def.Name, platform, arch)
			}
			if strings.Contains(src.URLTemplate, "{ext}") {
				for _, goos := range matchedGoos {
					if src.ExtMap[goos] == "" {
						t.Errorf("%s: sha256 pins %q but ext_map has no entry for Go OS %q — the expanded URL would have an empty {ext}", def.Name, platform, goos)
					}
				}
			}
		}
	}
}
