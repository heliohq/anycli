package hunter

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingKeyExitsOne(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "HUNTER_API_KEY is not set") {
		t.Errorf("stderr = %q, want missing-key message", errBuf.String())
	}
}

func TestCall_InjectsAPIKeyHeaderNeverQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"email":"a@b.com"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "account")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.APIKey != "key-123" {
		t.Errorf("X-API-KEY = %q, want key-123", got.APIKey)
	}
	if got.Auth != "" {
		t.Errorf("Authorization = %q, want empty (key must not ride Bearer)", got.Auth)
	}
	if strings.Contains(got.Query, "api_key") {
		t.Errorf("query = %q, must never contain api_key (leaks the key)", got.Query)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q", got.Accept)
	}
	if !strings.Contains(stdout, `"email"`) {
		t.Errorf("stdout = %q, want verbatim passthrough", stdout)
	}
}

func TestCall_401RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized,
		`{"errors":[{"id":"authentication_failed","code":401,"details":"Invalid or missing API key."}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true for 401")
	}
	if !strings.Contains(stderr, "authentication_failed") || !strings.Contains(stderr, "Invalid or missing API key") {
		t.Errorf("stderr = %q, want extracted id + details", stderr)
	}
}

func TestCall_403RateLimitIsPlainError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden,
		`{"errors":[{"id":"too_many_requests","details":"Rate limit reached."}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false for 403 (rate limit, not a bad key)")
	}
	if !strings.Contains(stderr, "Rate limit reached") {
		t.Errorf("stderr = %q, want rate-limit details", stderr)
	}
}

func TestCall_429QuotaExhaustedIsPlainError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests,
		`{"errors":[{"id":"usage_limit_reached","details":"Monthly quota exhausted."}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false for 429 (quota, not a bad key)")
	}
	if !strings.Contains(stderr, "Monthly quota exhausted") {
		t.Errorf("stderr = %q, want quota details", stderr)
	}
}

func TestCall_ErrorFallsBackToRawBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadGateway, `upstream boom`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "account")
	if !strings.Contains(stderr, "upstream boom") {
		t.Errorf("stderr = %q, want raw body fallback", stderr)
	}
}

func TestVerifier_202IsSuccessPassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusAccepted,
		`{"data":{"status":"accepted","result":"unknown"}}`, &got)
	defer srv.Close()

	result, stdout, stderr := runResult(t, srv, "email-verifier", "--email", "a@b.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (202 is a success passthrough); stderr=%q", result.ExitCode, stderr)
	}
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false for 202")
	}
	if !strings.Contains(stdout, `"status":"accepted"`) {
		t.Errorf("stdout = %q, want verbatim 202 body", stdout)
	}
	if got.Path != "/email-verifier" || got.Query != "email=a%40b.com" {
		t.Errorf("request = %s?%s, want /email-verifier?email=a%%40b.com", got.Path, got.Query)
	}
}

func TestUsageError_ExitsTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// Unknown subcommand is a cobra parse error → exit 1 via Failure path.
	// Missing a required flag likewise fails before any HTTP call.
	result, _, stderr := runResult(t, srv, "email-verifier")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for missing required --email (usage error)", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("made an HTTP call (%s) despite missing required flag", got.Method)
	}
	if !strings.Contains(stderr, "email") {
		t.Errorf("stderr = %q, want required-flag message", stderr)
	}
}

func TestBadFiltersJSON_ExitsTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "discover", "--filters", "{not json")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for malformed --filters JSON", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("made an HTTP call (%s) despite bad --filters JSON", got.Method)
	}
	if !strings.Contains(stderr, "filters") {
		t.Errorf("stderr = %q, want filters parse message", stderr)
	}
}
