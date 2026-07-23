package hubspot

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAccountEmitsBodyWithBearerAuth(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"portalId":12345678,"uiDomain":"app.hubspot.com"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "account")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/account-info/v3/details" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Fatalf("auth = %q", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Fatalf("accept = %q", got.Accept)
	}
	if strings.TrimSpace(stdout) != `{"portalId":12345678,"uiDomain":"app.hubspot.com"}` {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"account"}, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	// A missing bound token is a credential problem, not a flag-usage error:
	// exit 1 (never the usage exit 2) and CredentialRejected so the host
	// prompts a reconnect.
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatal("missing token should mark the credential rejected")
	}
	if !strings.Contains(errBuf.String(), EnvAccessToken) {
		t.Fatalf("stderr = %q", errBuf.String())
	}
}

func TestMissingTokenJSONEnvelopeKindIsCredential(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"--json", "account"}, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, errBuf.String())
	}
	// The envelope kind must agree with the exit code: exit 1 is never "usage"
	// (that is exit 2). A missing credential renders as "credential".
	if env.Error.Kind != "credential" {
		t.Fatalf("envelope kind = %q, want credential", env.Error.Kind)
	}
	if !strings.Contains(env.Error.Message, EnvAccessToken) {
		t.Fatalf("envelope message = %q", env.Error.Message)
	}
}

func TestAPIErrorIsExit1WithMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"status":"error","message":"Property values were not valid","category":"VALIDATION_ERROR"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "contact", "get", "1")
	if exit != 1 {
		t.Fatalf("exit = %d", exit)
	}
	if stdout != "" {
		t.Fatalf("stdout should be empty on error, got %q", stdout)
	}
	if !strings.Contains(stderr, "VALIDATION_ERROR") || !strings.Contains(stderr, "Property values were not valid") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"status":"error","message":"resource not found","category":"OBJECT_NOT_FOUND"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "--json", "deal", "get", "99")
	if exit != 1 {
		t.Fatalf("exit = %d", exit)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 404 {
		t.Fatalf("envelope = %+v", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"status":"error","message":"expired","category":"EXPIRED_AUTHENTICATION"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "account")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatal("401 should mark the credential rejected")
	}
}

func TestUsageErrorIsExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// create with no --prop is a usage error and must not reach the API.
	exit, _, stderr := run(t, srv, "contact", "create")
	if exit != 2 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "" {
		t.Fatalf("usage error must not hit the API, saw %s %s", got.Method, got.Path)
	}
	if !strings.Contains(stderr, "--prop") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestUnknownSubcommandIsExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "contact", "frobnicate")
	if exit != 2 {
		t.Fatalf("exit = %d", exit)
	}
}
