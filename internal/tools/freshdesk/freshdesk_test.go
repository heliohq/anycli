package freshdesk

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingAPIKey(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"agent", "me"}, map[string]string{EnvDomain: testDomain})
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

func TestExecute_MissingDomain(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"agent", "me"}, map[string]string{EnvAPIKey: testAPIKey})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvDomain+" is not set") {
		t.Errorf("stderr = %q, want the missing-domain message", errBuf.String())
	}
}

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{name: "bare subdomain", in: "acme", want: "acme.freshdesk.com"},
		{name: "full host", in: "acme.freshdesk.com", want: "acme.freshdesk.com"},
		{name: "https url", in: "https://acme.freshdesk.com", want: "acme.freshdesk.com"},
		{name: "http url with path", in: "http://acme.freshdesk.com/api/v2/tickets", want: "acme.freshdesk.com"},
		{name: "trailing slash", in: "acme.freshdesk.com/", want: "acme.freshdesk.com"},
		{name: "uppercase", in: "ACME", want: "acme.freshdesk.com"},
		{name: "whitespace", in: "  acme  ", want: "acme.freshdesk.com"},
		{name: "empty", in: "", err: true},
		{name: "whitespace only", in: "   ", err: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeDomain(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("normalizeDomain(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDomain(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("normalizeDomain(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveBaseURL_FromDomain(t *testing.T) {
	svc := &Service{}
	got, err := svc.resolveBaseURL("acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://acme.freshdesk.com/api/v2" {
		t.Errorf("resolveBaseURL = %q, want https://acme.freshdesk.com/api/v2", got)
	}
}

func TestBasicAuthHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":1,"contact":{"name":"Agent Smith"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "agent", "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Auth != wantAuth() {
		t.Errorf("Authorization = %q, want %q", got.Auth, wantAuth())
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: true},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
		{name: "bad request", status: http.StatusBadRequest, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"description":"nope","errors":[{"field":"status","message":"bad","code":"invalid_value"}]}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "agent", "me")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "nope") {
				t.Errorf("stderr = %q, want the provider description", stderr)
			}
		})
	}
}

func TestRateLimitSurfacesRetryAfter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"description":"throttled"}`, &got, [2]string{"Retry-After", "42"})
	defer srv.Close()

	code, _, stderr := run(t, srv, "agent", "me")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "42") || !strings.Contains(strings.ToLower(stderr), "retry after") {
		t.Errorf("stderr = %q, want Retry-After surfaced", stderr)
	}
}

func TestUsageErrorExitCode(t *testing.T) {
	// Missing required --id on `ticket get` is a cobra parse/usage error → exit 1
	// via cobra's error return. anycli's contract maps runtime/API + usage to
	// non-zero; assert it does not silently succeed.
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: "http://127.0.0.1:0", Out: &out, Err: &errBuf}
	env := map[string]string{EnvAPIKey: testAPIKey, EnvDomain: testDomain}
	result, err := svc.Execute(context.Background(), []string{"ticket", "get"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Errorf("exit code = 0, want non-zero for missing required flag")
	}
}
