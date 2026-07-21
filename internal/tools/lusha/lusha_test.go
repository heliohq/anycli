package lusha

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMissingAPIKey(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(nil, []string{"account", "usage"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "LUSHA_API_KEY is not set") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestAPIKeyHeaderInjected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"credits":{"remaining":10}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "account", "usage")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.APIKey != "key-123" {
		t.Errorf("api_key header = %q, want key-123", got.APIKey)
	}
	if got.Method != http.MethodGet || got.Path != "/account/usage" {
		t.Errorf("request = %s %s, want GET /account/usage", got.Method, got.Path)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"Invalid API key"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account", "usage")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true on 401")
	}
	if !strings.Contains(stderr, "Invalid API key") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRateLimitedIsAPIErrorNotRejection(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"message":"rate limit exceeded"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "--json", "account", "usage")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("CredentialRejected = true, want false on 429")
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"message":"bad filter"}`, &got)
	defer srv.Close()

	_, _, stderr := runResult(t, srv, "--json", "account", "usage")
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
	if env.Error.Kind != "api" || env.Error.Status != http.StatusBadRequest {
		t.Errorf("error envelope = %+v", env.Error)
	}
}

func TestUsageErrorExitCode2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// contact reveal with no --id is a usage error (exit 2), and must not hit
	// the network.
	result, _, _ := runResult(t, srv, "contact", "reveal")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("usage error should not reach the API, got %s %s", got.Method, got.Path)
	}
}

// strings_Builder is a tiny alias so the missing-key test can use a value type
// without importing strings.Builder under a colliding name.
type strings_Builder = strings.Builder
