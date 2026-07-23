package fullstory

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
	result, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAPIKey+" is not set") {
		t.Errorf("stderr = %q, want the missing-key message", errBuf.String())
	}
}

// TestRawBasicHeader is the load-bearing divergence assertion: FullStory's
// header is "Basic <raw-key>" — the key verbatim, NOT base64 and NOT Bearer.
func TestRawBasicHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"role":"USER"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Auth != "Basic na1.tok-123" {
		t.Errorf("Authorization = %q, want %q (raw key, no base64, no Bearer)", got.Auth, "Basic na1.tok-123")
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if got.Method != http.MethodGet || got.Path != "/me" {
		t.Errorf("request = %s %s, want GET /me", got.Method, got.Path)
	}
}

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
			srv := newServer(t, tc.status, `{"code":"bad","message":"nope"}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "me")
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

// TestQuota429SurfacesReason proves the monthly server-event quota reason is
// surfaced verbatim on exit 1 (not swallowed).
func TestQuota429SurfacesReason(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"code":"quota_exceeded","message":"monthly server event quota exceeded"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "event", "create", "--name", "x", "--uid", "u1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "monthly server event quota exceeded") {
		t.Errorf("stderr = %q, want the quota reason verbatim", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"code":"invalid","message":"bad input"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "me", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, `"kind":"api"`) || !strings.Contains(stderr, `"status":400`) {
		t.Errorf("stderr = %q, want JSON error envelope with kind=api and status=400", stderr)
	}
	if !strings.Contains(stderr, "bad input") {
		t.Errorf("stderr = %q, want the provider message inside the envelope", stderr)
	}
}

func TestUsageErrorExit2(t *testing.T) {
	// session list with neither --uid nor --email is a usage error → exit 2,
	// and must not issue any HTTP request.
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "session", "list")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("issued HTTP %s %s, want no request for a usage error", got.Method, got.Path)
	}
	if !strings.Contains(stderr, "requires --uid or --email") {
		t.Errorf("stderr = %q, want the usage message", stderr)
	}
}

func TestUsageErrorJSONEnvelope(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{}`, &capturedRequest{})
	defer srv.Close()
	code, _, stderr := run(t, srv, "session", "list", "--json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, `"kind":"usage"`) {
		t.Errorf("stderr = %q, want JSON usage envelope", stderr)
	}
}
