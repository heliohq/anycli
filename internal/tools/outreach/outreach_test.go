package outreach

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

type capturedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	Auth        string
	Accept      string
	ContentType string
	Body        []byte
}

func captureRequest(t *testing.T, r *http.Request) capturedRequest {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return capturedRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		RawQuery:    r.URL.RawQuery,
		Auth:        r.Header.Get("Authorization"),
		Accept:      r.Header.Get("Accept"),
		ContentType: r.Header.Get("Content-Type"),
		Body:        body,
	}
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func run(t *testing.T, server *httptest.Server, env map[string]string, args ...string) (int, string, string) {
	result, stdout, stderr := runResult(t, server, env, args...)
	return result.ExitCode, stdout, stderr
}

func runResult(t *testing.T, server *httptest.Server, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	svc := &Service{Out: &stdout, Err: &stderr}
	if server != nil {
		svc.BaseURL = server.URL
		svc.HC = server.Client()
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, stdout.String(), stderr.String()
}

func fullEnv() map[string]string {
	return map[string]string{EnvToken: "outreach-user-token"}
}

func jsonResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", mediaType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestExecuteRequiresAccessToken(t *testing.T) {
	result, _, stderr := runResult(t, nil, map[string]string{}, "prospect", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, EnvToken+" is not set") {
		t.Fatalf("stderr = %q, want missing-token message", stderr)
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	result, _, stderr := runResult(t, nil, map[string]string{}, "--json", "prospect", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, `"kind":"usage"`) || !strings.Contains(stderr, EnvToken) {
		t.Fatalf("stderr = %q, want JSON usage envelope", stderr)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "not-a-command")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q, want unknown-command error", stderr)
	}
}

func TestBearerAndJSONAPIHeadersOnEveryRequest(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "prospect", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Auth != "Bearer outreach-user-token" {
		t.Fatalf("Authorization = %q", got.Auth)
	}
	if got.Accept != mediaType {
		t.Fatalf("Accept = %q, want %q", got.Accept, mediaType)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"errors":[{"id":"invalidToken","title":"Unauthorized"}]}`, wantRejected: true},
		{name: "scope error 403", status: http.StatusForbidden, body: `{"errors":[{"id":"unauthorizedOauthScope","title":"Forbidden"}]}`, wantRejected: true},
		{name: "other 403", status: http.StatusForbidden, body: `{"errors":[{"id":"accessDenied","title":"Forbidden"}]}`, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"errors":[{"id":"rateLimit","title":"Too Many Requests"}]}`, wantRejected: false},
		{name: "server error", status: http.StatusInternalServerError, body: `{"errors":[{"title":"Internal"}]}`, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				jsonResponse(w, tc.status, tc.body)
			})
			defer server.Close()
			result, _, _ := runResult(t, server, fullEnv(), "prospect", "list")
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestAPIErrorSurfacesJSONAPIErrorsAndStatus(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusUnprocessableEntity,
			`{"errors":[{"id":"validationError","title":"Invalid","detail":"emails is invalid","source":{"pointer":"/data/attributes/emails"}}]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "prospect", "create", "--email", "bad")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	for _, want := range []string{"HTTP 422", "validationError", "emails is invalid", "/data/attributes/emails"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, missing %q", stderr, want)
		}
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusForbidden, `{"errors":[{"id":"unauthorizedOauthScope","title":"Forbidden"}]}`)
	})
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "--json", "prospect", "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, `"kind":"api"`) || !strings.Contains(stderr, `"status":403`) {
		t.Fatalf("stderr = %q, want api envelope with status", stderr)
	}
}
