package metaads

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPIErrorRendersEnvelopeAndRedactsToken(t *testing.T) {
	body := `{"error":{"message":"Unsupported get request. secret=meta-user-token","type":"GraphMethodException","code":100,"fbtrace_id":"Axyz"}}`
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusBadRequest, body)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "campaign", "get", "1")
	if code == 0 {
		t.Fatal("API error returned exit 0")
	}
	if !strings.Contains(stderr, "HTTP 400") || !strings.Contains(stderr, "GraphMethodException") {
		t.Fatalf("stderr = %q, want status + provider body", stderr)
	}
	if strings.Contains(stderr, "meta-user-token") {
		t.Fatalf("stderr leaked bearer token: %q", stderr)
	}
	if !strings.Contains(stderr, "[REDACTED]") {
		t.Fatalf("stderr = %q, want [REDACTED]", stderr)
	}
}

func TestOAuthExceptionClassifiedAsCredentialRejection(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
		wantHint     string
	}{
		{
			name:         "code 190 expired token",
			status:       http.StatusBadRequest,
			body:         `{"error":{"message":"Error validating access token","type":"OAuthException","code":190}}`,
			wantRejected: true,
			wantHint:     "reconnect Meta Ads",
		},
		{
			name:         "http 401",
			status:       http.StatusUnauthorized,
			body:         `{"error":{"message":"unauthorized"}}`,
			wantRejected: true,
			wantHint:     "reconnect Meta Ads",
		},
		{
			name:         "permission 403 not rejected",
			status:       http.StatusForbidden,
			body:         `{"error":{"message":"no permission","type":"OAuthException","code":200}}`,
			wantRejected: false,
			wantHint:     "required permission or ad-account access",
		},
		{
			name:         "rate limit not rejected",
			status:       http.StatusBadRequest,
			body:         `{"error":{"message":"rate","type":"OAuthException","code":17}}`,
			wantRejected: false,
			wantHint:     "rate/throttling limit reached",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				jsonResponse(w, tc.status, tc.body)
			})
			defer server.Close()

			result, _, stderr := runResult(t, server, fullEnv(), "accounts", "list")
			if result.ExitCode == 0 {
				t.Fatal("error returned exit 0")
			}
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if tc.wantHint != "" && !strings.Contains(stderr, tc.wantHint) {
				t.Errorf("stderr = %q, want hint %q", stderr, tc.wantHint)
			}
		})
	}
}
