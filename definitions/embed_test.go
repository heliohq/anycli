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
// and exposes a complete credential-injection shape.
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

// TestLoadBundled_GitHubCliShape pins the github definition's cli-type shape:
// it wraps the gh binary from a pinned github-release source and injects the
// minted token as GH_TOKEN.
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
	if def.Source == nil {
		t.Fatal("Source missing — the gh provisioning metadata must be declared")
	}
	if def.Source.Type != "github-release" || def.Source.Repo != "cli/cli" {
		t.Errorf("Source = %+v, want github-release cli/cli", def.Source)
	}
	if def.Source.Version != "2.63.0" {
		t.Errorf("Source.Version = %q, want pinned 2.63.0", def.Source.Version)
	}
	b := def.Auth.Credentials[0]
	if b.Source.Field != "access_token" {
		t.Errorf("field = %q, want access_token", b.Source.Field)
	}
	if b.Inject.Type != "env" || b.Inject.EnvVar != "GH_TOKEN" {
		t.Errorf("inject = %+v, want env GH_TOKEN", b.Inject)
	}
}
