package contacts

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// recordedRequest is one request the fake People API server saw.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

// route is a canned response for "METHOD /path".
type route struct {
	status int
	body   string
}

// fixture is a fake People API server: routes keyed by "METHOD /v1/...", every
// request recorded in order. resolve issues its two endpoint searches in
// parallel, so the recorder is mutex-guarded. Retry/warmup sleeps are recorded
// instead of slept so tests stay fast and deterministic.
type fixture struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	sleeps   []time.Duration
}

func newFixture(t *testing.T, routes map[string]route) *fixture {
	t.Helper()
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body.Bytes(),
		})
		f.mu.Unlock()
		rt, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"no route"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// requestsFor returns every recorded request matching method+path.
func (f *fixture) requestsFor(method, path string) []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedRequest
	for _, r := range f.requests {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// last returns the most recent request matching method+path.
func (f *fixture) last(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	got := f.requestsFor(method, path)
	if len(got) == 0 {
		t.Fatalf("no recorded request %s %s", method, path)
	}
	return got[len(got)-1]
}

func (f *fixture) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		BaseURL: f.srv.URL + "/v1",
		HC:      f.srv.Client(),
		Out:     &out,
		Err:     &errBuf,
		sleep: func(d time.Duration) {
			f.mu.Lock()
			f.sleeps = append(f.sleeps, d)
			f.mu.Unlock()
		},
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func (f *fixture) runOK(t *testing.T, args ...string) string {
	t.Helper()
	result, stdout, stderr := f.run(t, args...)
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	return stdout
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "CONTACTS_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"list", "explode"}, "unknown command"},
		{"unknown group subcommand", []string{"other", "explode"}, "unknown command"},
		{"bad sort", []string{"list", "--sort", "middle-name"}, "--sort must be"},
		{"search without query", []string{"search"}, "--query is required"},
		{"other search without query", []string{"other", "search"}, "--query is required"},
		{"get without id", []string{"get"}, "requires at least 1 arg"},
		{"resolve without name", []string{"resolve"}, "accepts 1 arg"},
		{"groups get without id", []string{"groups", "get"}, "accepts 1 arg"},
	}
	f := newFixture(t, map[string]route{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("argv failures must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestScopeHintOn403(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/people/me/connections": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 PERMISSION_DENIED must not reject the credential")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		providerStatus string
		wantRejected   bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "UNKNOWN", true},
		{"explicit unauthenticated status", http.StatusBadRequest, "UNAUTHENTICATED", true},
		{"permission denied", http.StatusForbidden, "PERMISSION_DENIED", false},
		{"rate limited", http.StatusTooManyRequests, "RESOURCE_EXHAUSTED", false},
		{"server failure", http.StatusInternalServerError, "INTERNAL", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 429/5xx are retried; repeat the same canned response so every
			// attempt sees it.
			f := newFixture(t, map[string]route{
				"GET /v1/people/me/connections": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "list")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestList_HumanAndJSON(t *testing.T) {
	body := `{"connections":[{"resourceName":"people/c1","names":[{"displayName":"Alice Adams","metadata":{"primary":true}}],"emailAddresses":[{"value":"alice@example.com","metadata":{"primary":true}}],"phoneNumbers":[{"value":"+15551234"}]}],"nextPageToken":"NEXT"}`
	f := newFixture(t, map[string]route{
		"GET /v1/people/me/connections": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "list", "--max", "50", "--sort", "last-modified")
	if !strings.Contains(stdout, "Alice Adams") || !strings.Contains(stdout, "alice@example.com") {
		t.Errorf("human output = %q, want name + email", stdout)
	}
	if !strings.Contains(stdout, "next page token: NEXT") {
		t.Errorf("human output = %q, want the next page token", stdout)
	}
	got := f.last(t, "GET", "/v1/people/me/connections")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}
	if !strings.Contains(got.Query, "personFields=names") || !strings.Contains(got.Query, "pageSize=50") {
		t.Errorf("query = %q, want personFields + pageSize", got.Query)
	}
	if !strings.Contains(got.Query, "sortOrder=LAST_MODIFIED_DESCENDING") {
		t.Errorf("query = %q, want the mapped sortOrder", got.Query)
	}

	stdout = f.runOK(t, "list", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestGet_SingleAndBatch(t *testing.T) {
	single := `{"resourceName":"people/c1","names":[{"displayName":"Bob Brown"}],"emailAddresses":[{"value":"bob@example.com"}],"organizations":[{"name":"Acme","title":"CTO"}]}`
	batch := `{"responses":[{"person":{"resourceName":"people/c1","names":[{"displayName":"Bob Brown"}],"emailAddresses":[{"value":"bob@example.com"}]}},{"person":{"resourceName":"people/c2","names":[{"displayName":"Cara Cole"}],"emailAddresses":[{"value":"cara@example.com"}]}}]}`
	f := newFixture(t, map[string]route{
		"GET /v1/people/c1":       {http.StatusOK, single},
		"GET /v1/people:batchGet": {http.StatusOK, batch},
	})

	stdout := f.runOK(t, "get", "people/c1")
	if !strings.Contains(stdout, "Bob Brown") || !strings.Contains(stdout, "Acme — CTO") {
		t.Errorf("single get = %q, want name + organization", stdout)
	}
	if len(f.requestsFor("GET", "/v1/people:batchGet")) != 0 {
		t.Error("single resource-name must not hit batchGet")
	}

	stdout = f.runOK(t, "get", "people/c1", "people/c2")
	if !strings.Contains(stdout, "people/c1") || !strings.Contains(stdout, "Cara Cole") {
		t.Errorf("batch get = %q, want both contacts", stdout)
	}
	got := f.last(t, "GET", "/v1/people:batchGet")
	if strings.Count(got.Query, "resourceNames=") != 2 {
		t.Errorf("query = %q, want two resourceNames", got.Query)
	}
}

func TestSearch_WarmupThenQuery(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/people:searchContacts": {http.StatusOK, `{"results":[{"person":{"resourceName":"people/c1","names":[{"displayName":"Dan Diaz"}],"emailAddresses":[{"value":"dan@example.com"}]}}]}`},
	})
	stdout := f.runOK(t, "search", "--query", "dan", "--max", "5")
	if !strings.Contains(stdout, "Dan Diaz") || !strings.Contains(stdout, "dan@example.com") {
		t.Errorf("search output = %q, want the match", stdout)
	}
	reqs := f.requestsFor("GET", "/v1/people:searchContacts")
	if len(reqs) != 2 {
		t.Fatalf("searchContacts calls = %d, want 2 (warmup + real)", len(reqs))
	}
	if !strings.Contains(reqs[0].Query, "query=&") && !strings.HasSuffix(reqs[0].Query, "query=") {
		t.Errorf("warmup query = %q, want an empty query param", reqs[0].Query)
	}
	if !strings.Contains(reqs[1].Query, "query=dan") {
		t.Errorf("real query = %q, want query=dan", reqs[1].Query)
	}
	if !strings.Contains(reqs[1].Query, "readMask=names") {
		t.Errorf("real query = %q, want readMask", reqs[1].Query)
	}
	if len(f.sleeps) == 0 || f.sleeps[len(f.sleeps)-1] != warmupDelay {
		t.Errorf("sleeps = %v, want a warmupDelay pause", f.sleeps)
	}
}

func TestSearch_MaxClampedTo30(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/people:searchContacts": {http.StatusOK, `{"results":[]}`},
	})
	stdout := f.runOK(t, "search", "--query", "x", "--max", "99")
	if !strings.Contains(stdout, "no contacts matched") {
		t.Errorf("output = %q, want the empty-result line", stdout)
	}
	real := f.requestsFor("GET", "/v1/people:searchContacts")[1]
	if !strings.Contains(real.Query, "pageSize=30") {
		t.Errorf("query = %q, want pageSize clamped to 30", real.Query)
	}
}

func TestOtherListAndSearch(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/otherContacts":        {http.StatusOK, `{"otherContacts":[{"resourceName":"otherContacts/o1","names":[{"displayName":"Eve East"}],"emailAddresses":[{"value":"eve@example.com"}]}]}`},
		"GET /v1/otherContacts:search": {http.StatusOK, `{"results":[{"person":{"resourceName":"otherContacts/o1","names":[{"displayName":"Eve East"}],"emailAddresses":[{"value":"eve@example.com"}]}}]}`},
	})
	stdout := f.runOK(t, "other", "list")
	if !strings.Contains(stdout, "Eve East") {
		t.Errorf("other list = %q, want the contact", stdout)
	}
	got := f.last(t, "GET", "/v1/otherContacts")
	if !strings.Contains(got.Query, "readMask=names") {
		t.Errorf("query = %q, want readMask", got.Query)
	}

	stdout = f.runOK(t, "other", "search", "--query", "eve")
	if !strings.Contains(stdout, "eve@example.com") {
		t.Errorf("other search = %q, want the match", stdout)
	}
	if len(f.requestsFor("GET", "/v1/otherContacts:search")) != 2 {
		t.Error("other search must warm up then query (2 calls)")
	}
}

func TestGroups(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/contactGroups":        {http.StatusOK, `{"contactGroups":[{"resourceName":"contactGroups/family","formattedName":"Family","groupType":"USER_CONTACT_GROUP","memberCount":4}]}`},
		"GET /v1/contactGroups/family": {http.StatusOK, `{"resourceName":"contactGroups/family","formattedName":"Family","groupType":"USER_CONTACT_GROUP","memberCount":4}`},
	})
	stdout := f.runOK(t, "groups", "list")
	if !strings.Contains(stdout, "Family") || !strings.Contains(stdout, "4 members") {
		t.Errorf("groups list = %q, want name + member count", stdout)
	}
	stdout = f.runOK(t, "groups", "get", "contactGroups/family")
	if !strings.Contains(stdout, "Members:      4") {
		t.Errorf("groups get = %q, want member count", stdout)
	}
}

func TestResolve_MergeDedupSource(t *testing.T) {
	// Alice appears in BOTH sources with the same primary email → one merged
	// row tagged my_contact (My Contacts wins). Frank appears only in Other.
	my := `{"results":[{"person":{"resourceName":"people/c1","names":[{"displayName":"Alice Adams"}],"emailAddresses":[{"value":"alice@example.com","metadata":{"primary":true}}],"organizations":[{"name":"Acme"}]}}]}`
	other := `{"results":[{"person":{"resourceName":"otherContacts/o1","names":[{"displayName":"Alice A"}],"emailAddresses":[{"value":"alice@example.com"}]}},{"person":{"resourceName":"otherContacts/o2","names":[{"displayName":"Frank Fox"}],"emailAddresses":[{"value":"frank@example.com"}]}}]}`
	f := newFixture(t, map[string]route{
		"GET /v1/people:searchContacts": {http.StatusOK, my},
		"GET /v1/otherContacts:search":  {http.StatusOK, other},
	})
	result, stdout, _ := f.run(t, "resolve", "alice", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	var out resolveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("resolve --json not parseable: %v (%q)", err, stdout)
	}
	if out.Count != 2 {
		t.Fatalf("count = %d, want 2 (alice deduped, frank kept)", out.Count)
	}
	if out.Matches[0].Source != sourceMyContact || out.Matches[0].Name != "Alice Adams" {
		t.Errorf("first match = %+v, want My Contact Alice Adams first", out.Matches[0])
	}
	if out.Matches[1].Source != sourceOtherContact || out.Matches[1].Name != "Frank Fox" {
		t.Errorf("second match = %+v, want Other Contact Frank Fox", out.Matches[1])
	}
}

func TestResolve_MissExitCode(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/people:searchContacts": {http.StatusOK, `{"results":[]}`},
		"GET /v1/otherContacts:search":  {http.StatusOK, `{"results":[]}`},
	})
	result, stdout, _ := f.run(t, "resolve", "ghost")
	if result.ExitCode != resolveMissExitCode {
		t.Errorf("exit code = %d, want %d for a zero-hit resolve", result.ExitCode, resolveMissExitCode)
	}
	if !strings.Contains(stdout, "no contacts matched") {
		t.Errorf("output = %q, want the no-match line", stdout)
	}
}

func TestResolve_SurfacesSearchError(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/people:searchContacts": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
		"GET /v1/otherContacts:search":  {http.StatusOK, `{"results":[]}`},
	})
	result, _, stderr := f.run(t, "resolve", "alice")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1 when a source errors", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") {
		t.Errorf("stderr = %q, want the surfaced error", stderr)
	}
}
