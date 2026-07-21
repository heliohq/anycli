package zohocrm

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMissingTokenExitsOneWithMessage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := runWithEnv(t, srv, map[string]string{}, "module", "list")
	if got.result.ExitCode != 1 {
		t.Fatalf("missing token exit = %d, want 1", got.result.ExitCode)
	}
	if !strings.Contains(got.stderr, EnvToken+" is not set") {
		t.Errorf("stderr = %q, want a %s-not-set message", got.stderr, EnvToken)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made without a token, got %d", len(reqs))
	}
}

func TestMissingTokenJSONError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := runWithEnv(t, srv, map[string]string{}, "module", "list", "--json")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(got.stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, got.stderr)
	}
	// A missing credential is a runtime credential failure (exit 1), not a
	// caller usage mistake (exit 2) — so the envelope kind must be "credential",
	// keeping the JSON kind aligned with the exit-code contract.
	if env.Error.Kind != "credential" || !strings.Contains(env.Error.Message, "not set") {
		t.Errorf("error envelope = %+v, want kind=credential", env.Error)
	}
}

func TestAPIErrorPlainAndExitOne(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"POST /crm/v8/Leads": {status: 400, body: `{"code":"INVALID_DATA","message":"invalid data","status":"error"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "create", "--module", "Leads", "--data", `{"x":"y"}`)
	if got.result.ExitCode != 1 {
		t.Fatalf("API error exit = %d, want 1", got.result.ExitCode)
	}
	if got.result.CredentialRejected {
		t.Error("a 400 INVALID_DATA must NOT reject the credential")
	}
	if !strings.Contains(got.stderr, "INVALID_DATA") || !strings.Contains(got.stderr, "HTTP 400") {
		t.Errorf("stderr should carry Zoho code + status: %s", got.stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /crm/v8/Leads/search": {status: 401, body: `{"code":"OAUTH_SCOPE_MISMATCH","message":"scope missing","status":"error"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "search", "--module", "Leads", "--word", "acme", "--json")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	// A scope mismatch is NOT a credential rejection — the token is valid.
	if got.result.CredentialRejected {
		t.Error("OAUTH_SCOPE_MISMATCH must not reject the credential")
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(got.stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, got.stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 401 {
		t.Errorf("envelope = %+v, want kind=api status=401", env.Error)
	}
}

func TestInvalidTokenRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /crm/v8/settings/modules": {status: 401, body: `{"code":"INVALID_TOKEN","message":"invalid oauth token","status":"error"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "module", "list")
	if got.result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", got.result.ExitCode)
	}
	if !got.result.CredentialRejected {
		t.Error("401 INVALID_TOKEN must reject the credential so the engine re-auths")
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "record", "frobnicate")
	if got.result.ExitCode != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", got.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call for an unknown subcommand, got %d", len(reqs))
	}
}

func TestBareGroupShowsHelpExitZero(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "record")
	if got.result.ExitCode != 0 {
		t.Fatalf("bare group exit = %d, want 0 (help)", got.result.ExitCode)
	}
}

// TestAuthHeaderScheme asserts every call uses the CRM Zoho-oauthtoken scheme.
func TestAuthHeaderScheme(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/org": {status: 200, body: `{"org":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	run(t, srv, "org", "get")
	req := findReq(reqs, http.MethodGet, "/crm/v8/org")
	if req.Auth != "Zoho-oauthtoken test-token" {
		t.Errorf("auth = %q, want Zoho-oauthtoken test-token", req.Auth)
	}
}
