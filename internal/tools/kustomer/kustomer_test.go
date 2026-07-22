package kustomer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records one request the fake server received.
type capturedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	Auth        string
	Accept      string
	ContentType string
	Body        []byte
}

// newServer is a single-route fake Kustomer server: it records the request and
// answers with the given status + body.
func newServer(t *testing.T, rec *capturedRequest, status int, respBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*rec = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			RawQuery:    r.URL.RawQuery,
			Auth:        r.Header.Get("Authorization"),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
}

// run drives one Service.Execute against the fake server, returning stdout,
// stderr, and the execution result.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf bytes.Buffer
	s := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := s.Execute(context.Background(), args, map[string]string{EnvToken: "tok-123"})
	if err != nil {
		t.Fatalf("Execute returned a Go error (should be nil): %v", err)
	}
	return out.String(), errBuf.String(), res
}

func TestResolveBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		orgName string
		want    string
	}{
		{"empty falls back to generic host", "", "https://api.kustomerapp.com/v1"},
		{"orgname builds pod-routed subdomain", "acme", "https://acme.api.kustomerapp.com/v1"},
		{"orgname is trimmed", "  acme  ", "https://acme.api.kustomerapp.com/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveBaseURL(tc.orgName); got != tc.want {
				t.Errorf("resolveBaseURL(%q) = %q, want %q", tc.orgName, got, tc.want)
			}
		})
	}
}

// captureRT records the outbound request URL and returns a canned 200 without
// touching the network — used to assert the base URL Execute builds from env.
type captureRT struct{ gotURL string }

func (c *captureRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.gotURL = r.URL.String()
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"data":{}}`)),
		Header:     make(http.Header),
	}, nil
}

func TestBaseURLResolvedFromOrgNameEnv(t *testing.T) {
	// With no BaseURL override, Execute must resolve the base from
	// KUSTOMER_ORG_NAME (the account_key credential) and build the pod-routed
	// org-subdomain host. Offline: a capturing RoundTripper records the URL.
	rt := &captureRT{}
	var out, errBuf bytes.Buffer
	s := &Service{HC: &http.Client{Transport: rt}, Out: &out, Err: &errBuf}
	res, err := s.Execute(context.Background(), []string{"customer", "get", "c1"},
		map[string]string{EnvToken: "tok", EnvOrgName: "acme"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if rt.gotURL != "https://acme.api.kustomerapp.com/v1/customers/c1" {
		t.Errorf("request URL = %q, want the pod-routed host built from KUSTOMER_ORG_NAME", rt.gotURL)
	}
}

func TestBaseURLFallsBackToGenericHostWhenOrgAbsent(t *testing.T) {
	// No KUSTOMER_ORG_NAME → the generic (default-pod) host, per the documented
	// org-absent fallback. Offline via a capturing RoundTripper.
	rt := &captureRT{}
	var out, errBuf bytes.Buffer
	s := &Service{HC: &http.Client{Transport: rt}, Out: &out, Err: &errBuf}
	res, _ := s.Execute(context.Background(), []string{"customer", "get", "c1"},
		map[string]string{EnvToken: "tok"})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if rt.gotURL != "https://api.kustomerapp.com/v1/customers/c1" {
		t.Errorf("request URL = %q, want the generic default-pod host", rt.gotURL)
	}
}

func TestMissingTokenFailsExit1(t *testing.T) {
	var out, errBuf bytes.Buffer
	s := &Service{Out: &out, Err: &errBuf}
	res, _ := s.Execute(context.Background(), []string{"customer", "get", "c1"}, map[string]string{})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "KUSTOMER_API_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token message", errBuf.String())
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	var out, errBuf bytes.Buffer
	s := &Service{Out: &out, Err: &errBuf}
	s.Execute(context.Background(), []string{"--json", "customer", "get", "c1"}, map[string]string{})
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, errBuf.String())
	}
	// A missing credential is exit 1 (runtime/credential failure), so the
	// envelope kind must not be the exit-2 "usage" class — it is "config".
	if env.Error.Kind != "config" {
		t.Errorf("kind = %q, want config", env.Error.Kind)
	}
}

func TestCustomerGet(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{"data":{"id":"c1","type":"customer"}}`)
	defer srv.Close()
	out, _, res := run(t, srv, "customer", "get", "c1")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if rec.Method != http.MethodGet || rec.Path != "/customers/c1" {
		t.Errorf("request = %s %s, want GET /customers/c1", rec.Method, rec.Path)
	}
	if rec.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", rec.Auth)
	}
	if rec.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", rec.Accept)
	}
	if strings.TrimSpace(out) != `{"data":{"id":"c1","type":"customer"}}` {
		t.Errorf("stdout = %q, want verbatim JSON:API passthrough", out)
	}
}

func TestCustomerGetByEmailEncodesValueInPath(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{"data":{}}`)
	defer srv.Close()
	_, _, res := run(t, srv, "customer", "get-by-email", "bob@acme.com")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	// The @ must be percent-encoded inside the email= path segment.
	if rec.Path != "/customers/email=bob@acme.com" && rec.Path != "/customers/email=bob%40acme.com" {
		t.Errorf("path = %q, want /customers/email=<encoded email>", rec.Path)
	}
	if !strings.Contains(rec.Path, "email=") {
		t.Errorf("path = %q, want the email= lookup segment", rec.Path)
	}
}

func TestCustomerConversationsWithPagination(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{"data":[]}`)
	defer srv.Close()
	_, _, res := run(t, srv, "customer", "conversations", "c1", "--page", "2", "--page-size", "50")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if rec.Path != "/customers/c1/conversations" {
		t.Errorf("path = %q, want /customers/c1/conversations", rec.Path)
	}
	if !strings.Contains(rec.RawQuery, "page=2") || !strings.Contains(rec.RawQuery, "pageSize=50") {
		t.Errorf("query = %q, want page=2 & pageSize=50", rec.RawQuery)
	}
}

func TestCustomerCreatePostsBody(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 201, `{"data":{"id":"c9"}}`)
	defer srv.Close()
	_, _, res := run(t, srv, "customer", "create", "--data", `{"name":"Acme"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if rec.Method != http.MethodPost || rec.Path != "/customers" {
		t.Errorf("request = %s %s, want POST /customers", rec.Method, rec.Path)
	}
	if rec.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.ContentType)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body, &got); err != nil || got["name"] != "Acme" {
		t.Errorf("body = %s, want {\"name\":\"Acme\"}", rec.Body)
	}
}

func TestConversationGetListCreateUpdate(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
	}{
		{"get", []string{"conversation", "get", "k1"}, http.MethodGet, "/conversations/k1"},
		{"list", []string{"conversation", "list"}, http.MethodGet, "/conversations"},
		{"create", []string{"conversation", "create", "--data", `{"x":1}`}, http.MethodPost, "/conversations"},
		{"update", []string{"conversation", "update", "k1", "--data", `{"status":"done"}`}, http.MethodPut, "/conversations/k1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec capturedRequest
			srv := newServer(t, &rec, 200, `{"data":{}}`)
			defer srv.Close()
			_, _, res := run(t, srv, tc.args...)
			if res.ExitCode != 0 {
				t.Fatalf("exit = %d, want 0", res.ExitCode)
			}
			if rec.Method != tc.wantMethod || rec.Path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", rec.Method, rec.Path, tc.wantMethod, tc.wantPath)
			}
		})
	}
}

func TestMessageListAndCreate(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
	}{
		{"list", []string{"message", "list", "k1"}, http.MethodGet, "/conversations/k1/messages"},
		{"create", []string{"message", "create", "k1", "--data", `{"body":"hi"}`}, http.MethodPost, "/conversations/k1/messages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec capturedRequest
			srv := newServer(t, &rec, 200, `{"data":{}}`)
			defer srv.Close()
			_, _, res := run(t, srv, tc.args...)
			if res.ExitCode != 0 {
				t.Fatalf("exit = %d, want 0", res.ExitCode)
			}
			if rec.Method != tc.wantMethod || rec.Path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", rec.Method, rec.Path, tc.wantMethod, tc.wantPath)
			}
		})
	}
}

func TestNoteListAndCreate(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
	}{
		{"list", []string{"note", "list", "k1"}, http.MethodGet, "/conversations/k1/notes"},
		{"create", []string{"note", "create", "k1", "--data", `{"note":"internal"}`}, http.MethodPost, "/conversations/k1/notes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec capturedRequest
			srv := newServer(t, &rec, 200, `{"data":{}}`)
			defer srv.Close()
			_, _, res := run(t, srv, tc.args...)
			if res.ExitCode != 0 {
				t.Fatalf("exit = %d, want 0", res.ExitCode)
			}
			if rec.Method != tc.wantMethod || rec.Path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", rec.Method, rec.Path, tc.wantMethod, tc.wantPath)
			}
		})
	}
}

func TestSearchCustomers(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{"data":[]}`)
	defer srv.Close()
	_, _, res := run(t, srv, "search", "customers", "--data", `{"and":[]}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if rec.Method != http.MethodPost || rec.Path != "/customers/search" {
		t.Errorf("request = %s %s, want POST /customers/search", rec.Method, rec.Path)
	}
}

func TestWriteBodyRequiredIsUsageError(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{"data":{}}`)
	defer srv.Close()
	_, errBuf, res := run(t, srv, "customer", "create")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", res.ExitCode)
	}
	if rec.Method != "" {
		t.Errorf("a request was made despite the missing body: %s %s", rec.Method, rec.Path)
	}
	if !strings.Contains(errBuf, "request body is required") {
		t.Errorf("stderr = %q, want required-body message", errBuf)
	}
}

func TestInvalidJSONBodyIsUsageError(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{"data":{}}`)
	defer srv.Close()
	_, _, res := run(t, srv, "customer", "create", "--data", `{not json`)
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", res.ExitCode)
	}
	if rec.Method != "" {
		t.Errorf("a request was made despite invalid JSON: %s %s", rec.Method, rec.Path)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{}`)
	defer srv.Close()
	_, _, res := run(t, srv, "customer", "nope")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", res.ExitCode)
	}
	if rec.Method != "" {
		t.Errorf("an unknown subcommand reached the API: %s %s", rec.Method, rec.Path)
	}
}

func TestAPIErrorExit1AndMessage(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 404, `{"errors":[{"title":"not_found","detail":"customer c1 not found"}]}`)
	defer srv.Close()
	_, errBuf, res := run(t, srv, "customer", "get", "c1")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 for API error", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Errorf("a 404 must not be classified as a credential rejection")
	}
	if !strings.Contains(errBuf, "HTTP 404") || !strings.Contains(errBuf, "customer c1 not found") {
		t.Errorf("stderr = %q, want status + provider detail", errBuf)
	}
}

func TestUnauthorizedIsCredentialRejection(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 401, `{"errors":[{"title":"unauthorized"}]}`)
	defer srv.Close()
	_, _, res := run(t, srv, "conversation", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Errorf("a 401 must be classified as a credential rejection")
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 422, `{"errors":[{"detail":"bad body"}]}`)
	defer srv.Close()
	_, errBuf, res := run(t, srv, "--json", "conversation", "create", "--data", `{}`)
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, errBuf)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("envelope = %+v, want kind=api status=422", env.Error)
	}
}

func TestDataAndFileMutuallyExclusive(t *testing.T) {
	var rec capturedRequest
	srv := newServer(t, &rec, 200, `{}`)
	defer srv.Close()
	_, errBuf, res := run(t, srv, "customer", "create", "--data", `{"a":1}`, "--file", "x.json")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
	if !strings.Contains(errBuf, "mutually exclusive") {
		t.Errorf("stderr = %q, want mutually-exclusive message", errBuf)
	}
}
