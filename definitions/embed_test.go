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
// and exposes the expected credential-injection shape (design 003 toolset).
func TestLoadBundled_ShippedDefinitions(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		envVars []string
	}{
		{"slack", "service", []string{"SLACK_BOT_TOKEN"}},
		{"notion", "service", []string{"NOTION_TOKEN"}},
		{"google", "service", []string{"GOOGLE_ACCESS_TOKEN"}},
		{"discord", "service", []string{"DISCORD_BOT_TOKEN"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def, err := LoadBundled(tc.name)
			if err != nil {
				t.Fatalf("LoadBundled(%q) failed: %v", tc.name, err)
			}
			if def.Name != tc.name {
				t.Errorf("Name = %q, want %q", def.Name, tc.name)
			}
			if def.Type != tc.typ {
				t.Errorf("Type = %q, want %q", def.Type, tc.typ)
			}
			if def.Auth == nil || len(def.Auth.Credentials) != len(tc.envVars) {
				t.Fatalf("want %d credential bindings, got %+v", len(tc.envVars), def.Auth)
			}
			for i, envVar := range tc.envVars {
				b := def.Auth.Credentials[i]
				if b.Inject.Type != "env" || b.Inject.EnvVar != envVar {
					t.Errorf("binding %d inject = %+v, want env %s", i, b.Inject, envVar)
				}
				if b.Source.VaultField == "" {
					t.Errorf("binding %d has no vault_field", i)
				}
			}
		})
	}
}
