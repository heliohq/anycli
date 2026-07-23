package brevo

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAccountGet_InjectsAPIKeyHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"email":"me@acme.com","companyName":"Acme"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "account", "get")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/account" {
		t.Errorf("request = %s %s, want GET /account", got.Method, got.Path)
	}
	if got.APIKey != "key-123" {
		t.Errorf("api-key header = %q, want key-123", got.APIKey)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"companyName"`) {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestMissingAPIKey_Exit1(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "BREVO_API_KEY") {
		t.Errorf("stderr = %q, want BREVO_API_KEY hint", errBuf.String())
	}
}

func TestAPIError_PlainRendersCodeAndMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"code":"invalid_parameter","message":"sender is invalid"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "account", "get")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid_parameter") || !strings.Contains(stderr, "sender is invalid") {
		t.Errorf("stderr = %q, want Brevo code + message", stderr)
	}
}

func TestAPIError_JSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"code":"invalid_parameter","message":"sender is invalid"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "account", "get", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Status  int    `json:"status"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Code != "invalid_parameter" {
		t.Errorf("error.code = %q, want invalid_parameter", env.Error.Code)
	}
	if env.Error.Message != "sender is invalid" {
		t.Errorf("error.message = %q", env.Error.Message)
	}
	if env.Error.Status != http.StatusBadRequest {
		t.Errorf("error.status = %d, want 400", env.Error.Status)
	}
}

func TestUnauthorized_RejectsCredential(t *testing.T) {
	srv := newServer(t, http.StatusUnauthorized, `{"code":"unauthorized","message":"Key not found"}`, new(capturedRequest))
	defer srv.Close()

	result, _, _ := runResult(t, srv, "account", "get")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("want CredentialRejected=true on 401")
	}
}

func TestUsageError_Exit2(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{}`, new(capturedRequest))
	defer srv.Close()

	// Unknown subcommand is a usage error -> exit 2.
	code, _, _ := run(t, srv, "contact", "frobnicate")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for unknown subcommand", code)
	}
}

func TestMissingRequiredFlag_Exit2(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{}`, new(capturedRequest))
	defer srv.Close()

	// contact get requires --id.
	code, _, _ := run(t, srv, "contact", "get")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing required flag", code)
	}
}
