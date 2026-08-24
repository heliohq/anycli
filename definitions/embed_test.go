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
// pinned exception: the policy-gate probe is credential-free by contract, so
// it must ship with NO auth block at all.
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
					t.Fatalf("gate-probe declares an auth block %+v; its definition pins it credential-free", def.Auth)
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

// toolCategories is the closed vocabulary a definition may declare. It is a
// shelving scheme for a host that lists tools to a person, so it stays small
// enough to read as a row of tabs: a tenth category is a decision, not a typo,
// and adding one here is where that decision gets made.
var toolCategories = map[string]bool{
	"Sales":           true,
	"Marketing":       true,
	"Social & Ads":    true,
	"Finance":         true,
	"Analytics":       true,
	"Support":         true,
	"Productivity":    true,
	"Forms & Signing": true,
	"Developer":       true,
}

// TestLoadBundled_TitleAndCategory pins the two display fields on every
// shipped definition. `name` is a wire identifier ("microsoft-onedrive",
// "billcom") and reads as one; a host that shows it to a person needs the name
// the vendor uses. Both are required — a definition that omits them lands
// unnamed and unshelved in every catalogue downstream, which no test after
// this one would catch.
func TestLoadBundled_TitleAndCategory(t *testing.T) {
	bundled, err := ListBundled()
	if err != nil {
		t.Fatalf("ListBundled failed: %v", err)
	}
	for _, def := range bundled {
		t.Run(def.Name, func(t *testing.T) {
			if def.Title == "" {
				t.Error("Title is empty")
			}
			if def.Category == "" {
				t.Fatal("Category is empty")
			}
			if !toolCategories[def.Category] {
				t.Errorf("Category = %q, which is not one of the shipped categories", def.Category)
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

// TestLoadBundled_SquareCredentialBindings pins Square's token and API origin
// to their repository-local environment bindings.
func TestLoadBundled_SquareCredentialBindings(t *testing.T) {
	def, err := LoadBundled("square")
	if err != nil {
		t.Fatalf("LoadBundled(square) failed: %v", err)
	}
	if def.Auth == nil || len(def.Auth.Credentials) != 2 {
		t.Fatalf("credentials = %+v, want access_token and base_url bindings", def.Auth)
	}
	baseURL := def.Auth.Credentials[1]
	if baseURL.Source.Field != "base_url" || baseURL.Inject.Type != "env" || baseURL.Inject.EnvVar != "SQUARE_BASE_URL" {
		t.Fatalf("base_url binding = %+v, want SQUARE_BASE_URL env injection", baseURL)
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
// service type so execTool's LoadBundled
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
	if def.Source == nil || def.Source.Type != "direct" {
		t.Errorf("Source = %+v, want a direct source (TestLoadBundled_LarkInstallsItself owns the rest)", def.Source)
	}
	want := []struct {
		field  string
		envVar string
	}{
		{field: "app_id", envVar: "LARKSUITE_CLI_APP_ID"},
		{field: "access_token", envVar: "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"},
		{field: "brand", envVar: "LARKSUITE_CLI_BRAND"},
		// The optional user identity: the host projects the
		// connection owner's user_access_token only when they granted it —
		// absent, the empty binding is skipped at inject and the CLI stays
		// bot-only. Present, lark-cli natively rides it for `--as user`.
		{field: "user_access_token", envVar: "LARKSUITE_CLI_USER_ACCESS_TOKEN"},
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
	// Dual identity is deliberate: bot by default, with `--as user` available
	// whenever a user token is present. Two before-rules hold that shape, and
	// each is pinned from the side it can fail on.
	//
	// DEFAULT_AS must be PRESENT. lark-cli does NOT default to the bot on its
	// own: its env credential source infers the default from which tokens are
	// injected, and a user access token alone flips every un-flagged command
	// (`im message send` included) to the user. Since this definition is what
	// injects LARKSUITE_CLI_USER_ACCESS_TOKEN, it is what owes the floor.
	//
	// STRICT_MODE must be ABSENT. lark-cli derives the usable identities from
	// the tokens it was given, so a bot-only lock would take `--as user` away
	// from a host that legitimately injected a user token.
	var sawDefaultAs bool
	for _, r := range def.Before {
		if r.Rule != "set_env" {
			continue
		}
		envVar, _ := r.Config["env_var"].(string)
		value, _ := r.Config["value"].(string)
		switch envVar {
		case "LARKSUITE_CLI_DEFAULT_AS":
			sawDefaultAs = true
			if value != "bot" {
				t.Errorf("LARKSUITE_CLI_DEFAULT_AS = %q, want bot — an injected user token must not silently change what un-flagged commands act as", value)
			}
		case "LARKSUITE_CLI_STRICT_MODE":
			t.Errorf("before rules set LARKSUITE_CLI_STRICT_MODE — dual identity (bot default + user token) must stay open")
		}
	}
	if !sawDefaultAs {
		t.Error("no before rule pins LARKSUITE_CLI_DEFAULT_AS; without it an injected user token makes every un-flagged command act as that person")
	}
}

// TestLoadBundled_LarkInstallsItself pins that the lark definition carries its
// own install contract: a direct source, a pinned version, and a sha256 for
// every platform it serves. That contract is what makes this definition the
// authority on which lark-cli runs — a source the resolver cannot install from
// leaves it with the PATH alone, so whatever the host image happens to ship
// decides the version instead, and nothing here can tell.
//
// A missing digest is what this catches. A digest that is merely STALE — left
// over from a previous version — still passes; only
// TestE2ERealLarkCliLazyInstall can see that, and it downloads.
func TestLoadBundled_LarkInstallsItself(t *testing.T) {
	def, err := LoadBundled("lark")
	if err != nil {
		t.Fatalf("LoadBundled(lark) failed: %v", err)
	}
	src := def.Source
	if src == nil || src.Type != "direct" {
		t.Fatalf("Source = %+v, want a direct source", src)
	}
	if src.URLTemplate == "" || src.Version == "" {
		t.Fatalf("Source = %+v, want both url_template and version", src)
	}
	// Every published archive is flat with the binary at its root, so one path
	// serves all platforms; {exe} is what keeps the Windows member name right.
	if src.BinaryPath != "lark-cli{exe}" {
		t.Errorf("BinaryPath = %q, want lark-cli{exe}", src.BinaryPath)
	}
	for _, platform := range []string{
		"darwin-amd64", "darwin-arm64",
		"linux-amd64", "linux-arm64",
		"windows-amd64", "windows-arm64",
	} {
		if src.SHA256[platform] == "" {
			t.Errorf("no sha256 pinned for %s; that platform silently falls back to PATH-only resolution", platform)
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

// TestLoadBundled_SupabaseShape pins the Supabase definition's CLI-wrapper
// shape: a service tool with an explicit command tree, backed by a pinned
// official Supabase CLI archive and an OAuth access token injected through the
// CLI's documented environment variable.
func TestLoadBundled_SupabaseShape(t *testing.T) {
	def, err := LoadBundled("supabase")
	if err != nil {
		t.Fatalf("LoadBundled(supabase) failed: %v", err)
	}
	if def.Type != "service" {
		t.Errorf("Type = %q, want service", def.Type)
	}
	if def.Binary != "supabase" {
		t.Errorf("Binary = %q, want supabase", def.Binary)
	}
	src := def.Source
	if src == nil {
		t.Fatal("Source missing — the Supabase CLI lazy-install source must be declared")
	}
	if src.Type != "direct" {
		t.Errorf("Source.Type = %q, want direct", src.Type)
	}
	if src.Version != "2.115.0" {
		t.Errorf("Source.Version = %q, want pinned 2.115.0", src.Version)
	}
	if src.URLTemplate == "" || src.BinaryPath == "" {
		t.Errorf("Source url_template/binary_path missing: %+v", src)
	}
	for _, platform := range []string{
		"darwin-arm64", "darwin-amd64",
		"linux-arm64", "linux-amd64",
		"windows-arm64", "windows-amd64",
	} {
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
	if binding.Source.Field != "access_token" {
		t.Errorf("field = %q, want access_token", binding.Source.Field)
	}
	if binding.Inject.Type != "env" || binding.Inject.EnvVar != "SUPABASE_ACCESS_TOKEN" {
		t.Errorf("inject = %+v, want env SUPABASE_ACCESS_TOKEN", binding.Inject)
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
