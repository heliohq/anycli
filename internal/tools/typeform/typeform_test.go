package typeform

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecuteMissingTokenExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	stdout, stderr, exit := run(t, srv, "", "me")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "TYPEFORM_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token message", stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("made %d requests with no token, want 0", len(reqs))
	}
}

// TestMissingTokenJSONEnvelopeUsageKind locks the deliberate taxonomy for the
// pre-parse credential-absent case: exit 1 (a runtime classification — the tool
// cannot run without the injected credential), rendered under --json as
// kind "usage". This matches the notion precedent across the tool batch; a
// missing token is never exit 2.
func TestMissingTokenJSONEnvelopeUsageKind(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, stderr, exit := run(t, srv, "", "--json", "me")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
	if !strings.Contains(env.Error.Message, "TYPEFORM_TOKEN is not set") {
		t.Errorf("message = %q, want missing-token message", env.Error.Message)
	}
}

func TestMeSuccessInjectsBearerAndEmitsJSON(t *testing.T) {
	var reqs []capturedRequest
	// The documented GET /me schema is exactly {alias, email, language}.
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {status: 200, body: `{"alias":"al","email":"a@b.co","language":"en"}`},
	})
	defer srv.Close()

	stdout, _, exit := run(t, srv, "tok-abc", "me")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	req := findReq(reqs, http.MethodGet, "/me")
	if req == nil {
		t.Fatal("no GET /me request recorded")
	}
	if req.Auth != "Bearer tok-abc" {
		t.Errorf("Authorization = %q, want %q", req.Auth, "Bearer tok-abc")
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.Accept)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, stdout)
	}
	if got["email"] != "a@b.co" {
		t.Errorf("email = %v, want a@b.co", got["email"])
	}
	if got["alias"] != "al" {
		t.Errorf("alias = %v, want al", got["alias"])
	}
}

func TestAPIErrorExit1WithMessage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms/f1": {status: 404, body: `{"code":"FORM_NOT_FOUND","description":"form not found"}`},
	})
	defer srv.Close()

	stdout, stderr, exit := run(t, srv, "tok", "form", "get", "f1")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on API error", stdout)
	}
	if !strings.Contains(stderr, "FORM_NOT_FOUND") || !strings.Contains(stderr, "form not found") {
		t.Errorf("stderr = %q, want Typeform code+description", stderr)
	}
	if !strings.Contains(stderr, "404") {
		t.Errorf("stderr = %q, want HTTP status", stderr)
	}
}

func TestAPIErrorJSONEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /forms/f1": {status: 400, body: `{"code":"VALIDATION_ERROR","description":"bad param"}`},
	})
	defer srv.Close()

	_, stderr, exit := run(t, srv, "tok", "--json", "form", "get", "f1")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != 400 {
		t.Errorf("status = %d, want 400", env.Error.Status)
	}
	if !strings.Contains(env.Error.Message, "VALIDATION_ERROR") {
		t.Errorf("message = %q, want provider code", env.Error.Message)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {status: 401, body: `{"code":"AUTHENTICATION_FAILED","description":"invalid token"}`},
	})
	defer srv.Close()

	// Call Execute directly to assert the credential-rejection classification on
	// the Result, so a token gateway would invalidate the token.
	svc := &Service{BaseURL: srv.URL, Out: &strings.Builder{}, Err: &strings.Builder{}}
	res, err := svc.Execute(t.Context(), []string{"me"}, map[string]string{EnvToken: "tok"})
	if err != nil {
		t.Fatalf("Execute transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("CredentialRejected = false, want true for a 401")
	}
}

func TestNon401DoesNotRejectCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {status: 403, body: `{"code":"FORBIDDEN","description":"insufficient scope"}`},
	})
	defer srv.Close()

	svc := &Service{BaseURL: srv.URL, Out: &strings.Builder{}, Err: &strings.Builder{}}
	res, err := svc.Execute(t.Context(), []string{"me"}, map[string]string{EnvToken: "tok"})
	if err != nil {
		t.Fatalf("Execute transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("CredentialRejected = true for a 403; a scope error must not invalidate the token")
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "form", "bogus")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", exit)
	}
	if len(reqs) != 0 {
		t.Errorf("made %d requests for a parse error, want 0", len(reqs))
	}
}

func TestBareGroupShowsHelpExit0(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, nil)
	defer srv.Close()

	_, _, exit := run(t, srv, "tok", "form")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 for a bare runnable group (help)", exit)
	}
}
