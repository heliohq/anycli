package sendgrid

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingKey(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"scopes"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "SENDGRID_API_KEY is not set") {
		t.Errorf("stderr = %q, want the missing-key message", errBuf.String())
	}
}

func TestScopes_HappyBearerAndPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusOK, body: `{"scopes":["mail.send","sender_verification_eligible"]}`}, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "scopes")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/scopes" {
		t.Errorf("request = %s %s, want GET /scopes", got.Method, got.Path)
	}
	if got.Auth != "Bearer SG.test-key" {
		t.Errorf("Authorization = %q, want Bearer SG.test-key", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"mail.send"`) {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

// TestCredentialRejectionClassification pins the load-bearing 401-vs-403 split:
// only a 401 rejects the credential; a 403 (scope / verified-sender) is a plain
// runtime error, never a credential rejection.
func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, serverResponse{status: tc.status, body: `{"errors":[{"message":"nope"}]}`}, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "scopes")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "nope") {
				t.Errorf("stderr = %q, want the provider message", stderr)
			}
		})
	}
}

func TestAPIError_JoinsMessages(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, serverResponse{status: http.StatusBadRequest, body: `{"errors":[{"field":"from","message":"from is required"},{"message":"subject is required"}]}`}, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "scopes")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "from is required") || !strings.Contains(stderr, "subject is required") {
		t.Errorf("stderr = %q, want both joined error messages", stderr)
	}
}

// TestRegionEU_SwapsHost proves --region eu selects the EU host when BaseURL is
// not overridden. It uses a real Service (no httptest) and asserts the resolved
// base directly, since the EU host is unreachable in unit tests.
func TestRegionEU_SwapsHost(t *testing.T) {
	svc := &Service{}
	if got := svc.baseURL("eu"); got != EUBaseURL {
		t.Errorf("baseURL(eu) = %q, want %q", got, EUBaseURL)
	}
	if got := svc.baseURL("global"); got != DefaultBaseURL {
		t.Errorf("baseURL(global) = %q, want %q", got, DefaultBaseURL)
	}
	// An explicit BaseURL (tests) always wins over region.
	svc.BaseURL = "http://127.0.0.1:1"
	if got := svc.baseURL("eu"); got != "http://127.0.0.1:1" {
		t.Errorf("baseURL with override = %q, want the override", got)
	}
}
