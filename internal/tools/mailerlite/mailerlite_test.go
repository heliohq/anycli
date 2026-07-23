package mailerlite

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingToken(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"subscriber", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "MAILERLITE_API_TOKEN is not set") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestExecute_MissingToken_JSONEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"--json", "subscriber", "list"}, map[string]string{})
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
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "MAILERLITE_API_TOKEN") {
		t.Errorf("envelope = %+v", env.Error)
	}
}

func TestExecute_Unauthenticated_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"Unauthenticated."}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "subscriber", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("expected CredentialRejected on 401, result = %+v", result)
	}
	if !strings.Contains(stderr, "Unauthenticated.") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestExecute_Unauthenticated_JSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"Unauthenticated."}`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "--json", "subscriber", "list")
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusUnauthorized {
		t.Errorf("envelope = %+v", env.Error)
	}
}

func TestExecute_ValidationError_422(t *testing.T) {
	var got capturedRequest
	body := `{"message":"The given data was invalid.","errors":{"email":["The email must be a valid email address."]}}`
	srv := newServer(t, http.StatusUnprocessableEntity, body, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "subscriber", "create", "--email", "bad")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("422 must not reject the credential, result = %+v", result)
	}
	if !strings.Contains(stderr, "valid email address") {
		t.Errorf("stderr should surface field errors, got %q", stderr)
	}
}

func TestExecute_UsageError_UnknownCommand(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"nonsense"}, map[string]string{EnvAPIToken: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 (usage)", result.ExitCode)
	}
}

func TestExecute_MissingRequiredFlag_IsUsageError(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	// subscriber create requires --email.
	result, err := svc.Execute(context.Background(), []string{"subscriber", "create"}, map[string]string{EnvAPIToken: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 (usage)", result.ExitCode)
	}
}

func TestExecute_AuthAndHeadersInjected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "subscriber", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Auth != "Bearer ml-tok-123" {
		t.Errorf("Authorization = %q, want Bearer ml-tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q", got.Accept)
	}
	if strings.TrimSpace(stdout) != `{"data":[]}` {
		t.Errorf("stdout = %q, want provider passthrough", stdout)
	}
}
