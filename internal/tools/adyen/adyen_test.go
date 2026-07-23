package adyen

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
	result, err := svc.Execute(context.Background(), []string{"management", "whoami"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "ADYEN_API_KEY is not set") {
		t.Errorf("stderr = %q, want the missing-key message", errBuf.String())
	}
}

// TestCredentialRejectionClassification pins the §2 rule: only 401 (auth
// failure) rejects the credential; a role/permission 403 and every other
// non-2xx is an ordinary passthrough API error at exit 1.
func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"status":401,"errorCode":"000","message":"Unauthorised","errorType":"security"}`, wantRejected: true},
		{name: "role forbidden", status: http.StatusForbidden, body: `{"status":403,"errorCode":"010","message":"The API credential is missing required roles","errorType":"security"}`, wantRejected: false},
		{name: "unprocessable", status: http.StatusUnprocessableEntity, body: `{"status":422,"errorCode":"702","message":"Invalid merchant account"}`, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"status":429,"message":"Too many requests"}`, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, body: `{"status":500,"message":"boom"}`, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, tc.body, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "management", "whoami")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "message") && !strings.Contains(stderr, "Unauthorised") && !strings.Contains(stderr, "roles") && !strings.Contains(stderr, "boom") && !strings.Contains(stderr, "Invalid") && !strings.Contains(stderr, "Too many") {
				t.Errorf("stderr = %q, want the provider message", stderr)
			}
		})
	}
}

// TestRoleForbiddenSurfacesErrorCode confirms the §2 actionable text: a 403
// passes through the {"error":...} envelope carrying Adyen's errorCode/message
// so the agent can add the missing role, never a rejection.
func TestRoleForbidden_SurfacesErrorCode(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden,
		`{"status":403,"errorCode":"010","message":"The API credential is missing required roles","errorType":"security"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "management", "merchant", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "010") || !strings.Contains(stderr, "missing required roles") {
		t.Errorf("stderr = %q, want errorCode 010 and the role message", stderr)
	}
}

func TestWhoami_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"id":"cred-123","companyName":"Acme","username":"ws@Company.Acme","roles":["Management API — Accounts read"]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "management", "whoami")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/me" {
		t.Errorf("request = %s %s, want GET /me", got.Method, got.Path)
	}
	if got.APIKey != "AQEyhmfxK..." {
		t.Errorf("X-API-Key = %q, want the raw key (no Bearer)", got.APIKey)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"id":"cred-123"`) {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

// TestNoBearerPrefix guards the DESIGN §2 rule: the key is sent raw, never with
// an Authorization: Bearer header.
func TestWhoami_NoAuthorizationHeader(t *testing.T) {
	srv := newServerAsserting(t, func(r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization header set to %q, want empty (Adyen uses raw X-API-Key)", r.Header.Get("Authorization"))
		}
	}, `{"id":"cred-1"}`)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "whoami")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
}
