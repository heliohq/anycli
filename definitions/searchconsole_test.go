package definitions

import "testing"

// TestLoadBundled_SearchConsoleCredentialBinding pins the search-console
// definition's service shape and its single access-token env binding — the
// wire contract with the host's credential projection (a drifted field or env
// var name means no injection at runtime).
func TestLoadBundled_SearchConsoleCredentialBinding(t *testing.T) {
	def, err := LoadBundled("search-console")
	if err != nil {
		t.Fatalf("LoadBundled(search-console) failed: %v", err)
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
	if binding.Inject.Type != "env" || binding.Inject.EnvVar != "SEARCH_CONSOLE_ACCESS_TOKEN" {
		t.Errorf("inject = %+v, want env SEARCH_CONSOLE_ACCESS_TOKEN", binding.Inject)
	}
}
