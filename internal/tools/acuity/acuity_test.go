package acuity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMissingTokenIsRuntimeFailure(t *testing.T) {
	result, stdout, stderr := runNoToken(t, "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("missing token must not be classified as credential rejection")
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, EnvAccessToken) {
		t.Errorf("stderr = %q, want mention of %s", stderr, EnvAccessToken)
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	result, _, stderr := runNoToken(t, "me", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Message == "" {
		t.Errorf("envelope missing message: %s", stderr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"status_code":401,"message":"Unauthorized","error":"unauthorized"}`, &got)
	defer srv.Close()

	result, stdout, stderr := runResult(t, srv, "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("401 must set CredentialRejected")
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	if !strings.Contains(stderr, "Unauthorized") {
		t.Errorf("stderr = %q, want provider message", stderr)
	}
}

func TestGenericAPIErrorIsExit1NoRejection(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 400, `{"status_code":400,"message":"Invalid appointment type","error":"invalid_appointment_type"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "appointment", "get", "42")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("non-401 must not reject the credential")
	}
	if !strings.Contains(stderr, "Invalid appointment type") {
		t.Errorf("stderr = %q, want provider message", stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 404, `{"status_code":404,"message":"Not found","error":"not_found"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "appointment", "get", "42", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != 404 {
		t.Errorf("status = %d, want 404", env.Error.Status)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "bogus")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("unknown subcommand must not reach the API (saw %s %s)", got.Method, got.Path)
	}
}

func TestMissingRequiredFlagIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	// create requires --type-id, --datetime, --first-name, --last-name.
	result, _, _ := runResult(t, srv, "appointment", "create", "--first-name", "Jane")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing required flags", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("usage error must not reach the API")
	}
}

func TestBearerAuthAndAcceptHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"email":"biz@example.com"}`, &got)
	defer srv.Close()

	run(t, srv, "me")
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
}
