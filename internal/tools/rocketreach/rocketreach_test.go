package rocketreach

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingKey(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "ROCKETREACH_API_KEY is not set") {
		t.Errorf("stderr = %q, want the missing-key message", errBuf.String())
	}
}

func TestExecute_MissingKey_JSON(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "ROCKETREACH_API_KEY is not set") {
		t.Errorf("envelope = %+v, want kind=usage with the missing-key message", env.Error)
	}
}

func TestAccount_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":123,"email":"me@acme.com","first_name":"Jane","credit_usage":[{"credit_type":"lookup","remaining":"inf"}]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "account")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/api/v2/account/" {
		t.Errorf("request = %s %s, want GET /api/v2/account/", got.Method, got.Path)
	}
	assertAPIKey(t, got)
	if !strings.Contains(stdout, `"email":"me@acme.com"`) {
		t.Errorf("stdout = %q, want the provider JSON passthrough", stdout)
	}
}

func TestAccount_APIError_CredentialRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"detail":"Invalid API key"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 for an API error", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true on a 401")
	}
	if !strings.Contains(stderr, "401") {
		t.Errorf("stderr = %q, want the HTTP status", stderr)
	}
}

func TestAccount_APIError_RateLimited_NotRejected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"detail":"rate limit exceeded"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false on a 429")
	}
	if !strings.Contains(stderr, "rate limit") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
}

func TestAccount_JSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"detail":"forbidden"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "account", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusForbidden {
		t.Errorf("envelope = %+v, want kind=api status=403", env.Error)
	}
}

func TestUnknownSubcommand_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "person", "destroy")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for an unknown subcommand", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want an unknown-command error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for an unknown subcommand, got %s", got.Path)
	}
}
