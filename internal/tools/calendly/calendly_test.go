package calendly

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMeSendsBearerAndEmitsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"resource":{"uri":"u1"}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/users/me" {
		t.Errorf("request = %s %s, want GET /users/me", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if strings.TrimSpace(stdout) != `{"resource":{"uri":"u1"}}` {
		t.Errorf("stdout = %q, want provider body passthrough", stdout)
	}
}

func TestMissingTokenIsExit1(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "CALENDLY_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token message", errBuf.String())
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	_, err := svc.Execute(t.Context(), []string{"me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestAPIErrorIsExit1WithStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"title":"Resource Not Found","message":"no such user"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "me")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "HTTP 404") || !strings.Contains(stderr, "Resource Not Found") {
		t.Errorf("stderr = %q, want HTTP 404 + title", stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"title":"Permission Denied","message":"paid plan required"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "me", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr not JSON: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 403 {
		t.Errorf("envelope = %+v, want kind=api status=403", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"title":"Unauthorized","message":"bad token"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true for 401")
	}
}

func TestUnknownSubcommandIsExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "bogus")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for unknown subcommand", code)
	}
}
