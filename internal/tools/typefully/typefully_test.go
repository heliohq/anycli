package typefully

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "TYPEFULLY_API_KEY is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestMe_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":12345,"name":"Alice","email":"a@example.com"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/me" {
		t.Errorf("request = %s %s, want GET /me", got.Method, got.Path)
	}
	if got.Auth != "Bearer key-123" {
		t.Errorf("Authorization = %q, want Bearer key-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"id":12345`) {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
		wantKind     string
	}{
		{name: "401 unauthorized", status: http.StatusUnauthorized, body: `{"detail":"Invalid API key"}`, wantRejected: true, wantKind: "api"},
		{name: "403 auth-shaped", status: http.StatusForbidden, body: `{"detail":"Missing or invalid authentication"}`, wantRejected: true, wantKind: "api"},
		{name: "403 permission-scoped", status: http.StatusForbidden, body: `{"detail":"Insufficient permissions for this social set"}`, wantRejected: false, wantKind: "permission"},
		{name: "429 rate limit", status: http.StatusTooManyRequests, body: `{"detail":"Too many requests"}`, wantRejected: false, wantKind: "rate_limit"},
		{name: "500 server", status: http.StatusInternalServerError, body: `{"detail":"boom"}`, wantRejected: false, wantKind: "api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, tc.body, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "me")
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if len(stderr) == 0 {
				t.Error("expected an error message on stderr")
			}
		})
	}
}

func TestPermission403_JSONEnvelopeKind(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"detail":"Insufficient permissions"}`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "--json", "me")
	if !strings.Contains(stderr, `"kind":"permission"`) {
		t.Errorf("stderr = %q, want kind permission in JSON envelope", stderr)
	}
	if !strings.Contains(stderr, `"status":403`) {
		t.Errorf("stderr = %q, want status 403 in JSON envelope", stderr)
	}
}

func TestRateLimit_ResetHint(t *testing.T) {
	var got capturedRequest
	srv := newHeaderServer(t, http.StatusTooManyRequests, `{"detail":"slow down"}`, map[string]string{"X-RateLimit-Reset": "1750000000"}, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "me")
	if result.CredentialRejected {
		t.Error("429 must not reject the credential")
	}
	if !strings.Contains(stderr, "1750000000") {
		t.Errorf("stderr = %q, want the rate-limit reset hint", stderr)
	}
}

func TestUnknownSubcommand_ExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "bogus")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for unknown subcommand", code)
	}
}

func TestMissingRequiredFlag_ExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// social-set get requires --social-set.
	code, _, _ := run(t, srv, "social-set", "get")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for missing required flag", code)
	}
}
