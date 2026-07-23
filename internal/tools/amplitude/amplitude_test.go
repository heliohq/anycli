package amplitude

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// (a) Basic auth header is set on every request from the injected credentials.
func TestBasicAuthHeaderIsSet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	res, _, stderr := run(t, srv, "events", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, stderr)
	}
	if got.Auth != wantAuthHeader() {
		t.Errorf("Authorization = %q, want %q", got.Auth, wantAuthHeader())
	}
}

// (b) --region eu selects the EU host; default selects the US host. Uses a
// capturing RoundTripper (no BaseURL override) so the real host resolution is
// exercised.
func TestRegionSelectsHost(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantHost string
	}{
		{"default us", []string{"events", "list"}, "amplitude.com"},
		{"explicit eu", []string{"events", "list", "--region", "eu"}, "analytics.eu.amplitude.com"},
		{"explicit us", []string{"events", "list", "--region", "us"}, "amplitude.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &capturingRT{status: http.StatusOK, body: `{"data":[]}`}
			svc := &Service{HC: &http.Client{Transport: rt}}
			res, err := svc.Execute(context.Background(), tc.args, map[string]string{EnvCredentials: testCreds})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if res.ExitCode != 0 {
				t.Fatalf("exit = %d", res.ExitCode)
			}
			if rt.got.URL.Scheme != "https" {
				t.Errorf("scheme = %q, want https", rt.got.URL.Scheme)
			}
			if rt.got.URL.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", rt.got.URL.Host, tc.wantHost)
			}
		})
	}
}

// (g) Malformed credentials → exit 2, never rejected, secret never echoed.
func TestMalformedCredentialsExit2(t *testing.T) {
	cases := []struct {
		name  string
		creds string
	}{
		{"no colon", "justonevalue"},
		{"empty secret", "apikey:"},
		{"empty key", ":secret"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf strings.Builder
			svc := &Service{Out: &out, Err: &errBuf}
			res, err := svc.Execute(context.Background(), []string{"events", "list"}, map[string]string{EnvCredentials: tc.creds})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if res.ExitCode != 2 {
				t.Errorf("exit = %d, want 2", res.ExitCode)
			}
			if res.CredentialRejected {
				t.Error("malformed credentials must not set CredentialRejected")
			}
			// The guidance is static and never interpolates the injected value
			// (secret non-leak on a live 401 is covered in
			// TestUnauthorizedClassification with a distinctive secret).
		})
	}
}

// (g/h) Region-aware 401 classification and secret non-leak.
func TestUnauthorizedClassification(t *testing.T) {
	const body = `{"error":"invalid api key"}`

	t.Run("explicit region 401 rejects credential", func(t *testing.T) {
		var got capturedRequest
		srv := newServer(t, http.StatusUnauthorized, body, &got)
		defer srv.Close()

		res, _, stderr := run(t, srv, "events", "list", "--region", "us")
		if res.ExitCode != 1 {
			t.Errorf("exit = %d, want 1", res.ExitCode)
		}
		if !res.CredentialRejected {
			t.Error("explicit-region 401 must set CredentialRejected")
		}
		if strings.Contains(stderr, "--region eu") {
			t.Errorf("explicit-region 401 must NOT carry the EU retry hint: %q", stderr)
		}
	})

	t.Run("default region 401 does not reject and hints eu", func(t *testing.T) {
		var got capturedRequest
		srv := newServer(t, http.StatusUnauthorized, body, &got)
		defer srv.Close()

		res, _, stderr := run(t, srv, "events", "list")
		if res.ExitCode != 1 {
			t.Errorf("exit = %d, want 1", res.ExitCode)
		}
		if res.CredentialRejected {
			t.Error("default-region 401 must NOT set CredentialRejected (could be an EU key)")
		}
		if !strings.Contains(stderr, "--region eu") {
			t.Errorf("default-region 401 must carry the --region eu retry hint: %q", stderr)
		}
	})

	t.Run("explicit eu region 401 rejects credential", func(t *testing.T) {
		var got capturedRequest
		srv := newServer(t, http.StatusUnauthorized, body, &got)
		defer srv.Close()

		res, _, _ := run(t, srv, "events", "list", "--region", "eu")
		if res.ExitCode != 1 || !res.CredentialRejected {
			t.Errorf("explicit eu 401: exit=%d rejected=%v, want 1/true", res.ExitCode, res.CredentialRejected)
		}
	})

	t.Run("secret never leaks in error", func(t *testing.T) {
		var got capturedRequest
		srv := newServer(t, http.StatusUnauthorized, body, &got)
		defer srv.Close()

		_, _, stderr := run(t, srv, "events", "list")
		if strings.Contains(stderr, testSecretKey) {
			t.Errorf("error string leaked the secret key: %q", stderr)
		}
	})
}

// A non-401 failure is exit 1 but never rejects the credential.
func TestNon401FailureDoesNotReject(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"error":"rate limited"}`, &got)
	defer srv.Close()

	res, _, _ := run(t, srv, "events", "list", "--region", "us")
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("429 must not set CredentialRejected")
	}
}

// --json renders the structured error envelope for API failures.
func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"error":"bad request"}`, &got)
	defer srv.Close()

	res, _, stderr := run(t, srv, "events", "list", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(stderr, `"kind":"api"`) || !strings.Contains(stderr, `"status":400`) {
		t.Errorf("json error envelope missing kind/status: %q", stderr)
	}
}

// A bad --region value is a usage error (exit 2).
func TestBadRegionExit2(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"events", "list", "--region", "apac"}, map[string]string{EnvCredentials: testCreds})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 2 {
		t.Errorf("exit = %d, want 2", res.ExitCode)
	}
}

// Sanity: the execution package classifies our rejected error via errors.As.
func TestRejectedErrorIsClassified(t *testing.T) {
	e := classifyCredentialError(true, http.StatusUnauthorized, &apiError{msg: "boom"})
	if !execution.IsCredentialRejected(e) {
		t.Fatal("explicit-region 401 should be classified as credential rejection")
	}
}
