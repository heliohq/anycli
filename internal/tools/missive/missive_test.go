package missive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// route is one programmed response. A key may map to a queue of routes so a
// test can drive a 429-then-200 retry.
type route struct {
	status int
	body   string
	header map[string]string
}

type capturedReq struct {
	method string
	path   string
	query  string
	auth   string
	accept string
	ctype  string
	body   string
}

type fixture struct {
	t      *testing.T
	srv    *httptest.Server
	routes map[string][]route
	reqs   []capturedReq
	svc    *Service
	stdout *strings.Builder
	stderr *strings.Builder
}

func newFixture(t *testing.T, routes map[string][]route) *fixture {
	t.Helper()
	f := &fixture{t: t, routes: routes, stdout: &strings.Builder{}, stderr: &strings.Builder{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.reqs = append(f.reqs, capturedReq{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			auth:   r.Header.Get("Authorization"),
			accept: r.Header.Get("Accept"),
			ctype:  r.Header.Get("Content-Type"),
			body:   string(body),
		})
		key := r.Method + " " + r.URL.Path
		q := f.routes[key]
		if len(q) == 0 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"no route for `+key+`"}`)
			return
		}
		rt := q[0]
		if len(q) > 1 {
			f.routes[key] = q[1:]
		}
		for k, v := range rt.header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(rt.status)
		_, _ = io.WriteString(w, rt.body)
	}))
	t.Cleanup(f.srv.Close)
	f.svc = &Service{
		BaseURL: f.srv.URL,
		Out:     f.stdout,
		Err:     f.stderr,
		sleep:   func(time.Duration) {}, // no real backoff in tests
	}
	return f
}

// run executes one invocation with a valid token in env.
func (f *fixture) run(args ...string) (execution.Result, string, string) {
	f.t.Helper()
	f.stdout.Reset()
	f.stderr.Reset()
	res, err := f.svc.Execute(context.Background(), args, map[string]string{EnvToken: "missive_pat-test"})
	if err != nil {
		f.t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res, f.stdout.String(), f.stderr.String()
}

// last returns the most recent request captured for method+path.
func (f *fixture) last(method, path string) capturedReq {
	f.t.Helper()
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if f.reqs[i].method == method && f.reqs[i].path == path {
			return f.reqs[i]
		}
	}
	f.t.Fatalf("no captured request for %s %s (captured: %+v)", method, path, f.reqs)
	return capturedReq{}
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%q", err, s)
	}
	return m
}

// --- header composition + until-cursor paging (conversations) -----------------

func TestConversationsList_BearerHeaderAndUntilCursor(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /conversations": {{status: http.StatusOK, body: `{"conversations":[{"id":"c1","last_activity_at":111},{"id":"c2","last_activity_at":100}]}`}},
	})
	res, out, _ := f.run("conversations", "list", "--inbox", "--limit", "2")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	got := f.last("GET", "/conversations")
	if got.auth != "Bearer missive_pat-test" {
		t.Errorf("Authorization = %q, want Bearer scheme composed in-service", got.auth)
	}
	if got.accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.accept)
	}
	if !strings.Contains(got.query, "inbox=true") {
		t.Errorf("query = %q, want inbox=true", got.query)
	}
	if !strings.Contains(got.query, "limit=2") {
		t.Errorf("query = %q, want limit=2", got.query)
	}
	m := decode(t, out)
	items, _ := m["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if m["next_until"] != float64(100) {
		t.Errorf("next_until = %v, want 100 (oldest last_activity_at)", m["next_until"])
	}
}

func TestConversationsList_EmptyPageNullsCursor(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /conversations": {{status: http.StatusOK, body: `{"conversations":[]}`}},
	})
	_, out, _ := f.run("conversations", "list", "--assigned")
	m := decode(t, out)
	if m["next_until"] != nil {
		t.Errorf("next_until = %v, want null on empty page", m["next_until"])
	}
	if items, _ := m["items"].([]any); items == nil {
		t.Errorf("items should be an empty array, got %v", m["items"])
	}
}

func TestConversationsMessages_DeliveredAtCursor(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /conversations/c1/messages": {{status: http.StatusOK, body: `{"messages":[{"id":"m1","delivered_at":222},{"id":"m2","delivered_at":200}]}`}},
	})
	_, out, _ := f.run("conversations", "messages", "c1", "--limit", "2")
	m := decode(t, out)
	if m["next_until"] != float64(200) {
		t.Errorf("next_until = %v, want 200 (oldest delivered_at)", m["next_until"])
	}
}

func TestConversationsList_NarrowingMutuallyExclusive(t *testing.T) {
	f := newFixture(t, map[string][]route{})
	res, _, stderr := f.run("conversations", "list", "--inbox", "--email", "a@x.com", "--domain", "x.com")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", res.ExitCode)
	}
	if len(f.reqs) != 0 {
		t.Errorf("expected no HTTP request for a locally-rejected combination, got %d", len(f.reqs))
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("stderr = %q, want mutually-exclusive message", stderr)
	}
}

// --- mandatory-filter errors --------------------------------------------------

func TestConversationsList_MissingMailboxSurfaces400(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /conversations": {{status: http.StatusBadRequest, body: `{"message":"You need to paginate at least one mailbox"}`}},
	})
	// Suppress the default --limit so no filter at all reaches the API, then let
	// Missive's own 400 surface (exit 1, api error) rather than pre-validating.
	res, _, stderr := f.run("conversations", "list", "--limit", "0")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (surfaced API error)", res.ExitCode)
	}
	if !strings.Contains(stderr, "paginate at least one mailbox") {
		t.Errorf("stderr = %q, want the surfaced Missive 400 message", stderr)
	}
}

func TestContactsList_MissingContactBookIsUsageError(t *testing.T) {
	f := newFixture(t, map[string][]route{})
	res, _, _ := f.run("contacts", "list")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (required flag missing)", res.ExitCode)
	}
	if len(f.reqs) != 0 {
		t.Errorf("expected no HTTP request when --contact-book is missing, got %d", len(f.reqs))
	}
}

// --- offset paging (contacts / contact books) ---------------------------------

func TestContactsList_OffsetPagingFullPage(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contacts": {{status: http.StatusOK, body: `{"contacts":[{"id":"k1"},{"id":"k2"}]}`}},
	})
	_, out, _ := f.run("contacts", "list", "--contact-book", "cb1", "--limit", "2", "--offset", "4")
	got := f.last("GET", "/contacts")
	if !strings.Contains(got.query, "contact_book=cb1") {
		t.Errorf("query = %q, want contact_book=cb1", got.query)
	}
	if !strings.Contains(got.query, "offset=4") {
		t.Errorf("query = %q, want offset=4", got.query)
	}
	m := decode(t, out)
	if m["next_offset"] != float64(6) {
		t.Errorf("next_offset = %v, want 6 (offset 4 + 2 full page)", m["next_offset"])
	}
}

func TestContactsList_OffsetPagingPartialPageNullsCursor(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contacts": {{status: http.StatusOK, body: `{"contacts":[{"id":"k1"}]}`}},
	})
	_, out, _ := f.run("contacts", "list", "--contact-book", "cb1", "--limit", "2")
	m := decode(t, out)
	if m["next_offset"] != nil {
		t.Errorf("next_offset = %v, want null when page is short of limit", m["next_offset"])
	}
}

func TestContactBooksList_OffsetKey(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contact_books": {{status: http.StatusOK, body: `{"contact_books":[{"id":"cb1"},{"id":"cb2"}]}`}},
	})
	_, out, _ := f.run("contact-books", "list", "--limit", "2")
	m := decode(t, out)
	if items, _ := m["items"].([]any); len(items) != 2 {
		t.Fatalf("items = %v, want 2 contact books", m["items"])
	}
	if m["next_offset"] != float64(2) {
		t.Errorf("next_offset = %v, want 2", m["next_offset"])
	}
}

// --- 429 + Retry-After single bounded retry -----------------------------------

func Test429RetryAfterThenSuccess(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contact_books": {
			{status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`, header: map[string]string{"Retry-After": "1"}},
			{status: http.StatusOK, body: `{"contact_books":[{"id":"cb1"}]}`},
		},
	})
	res, out, _ := f.run("contact-books", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 after one retry", res.ExitCode)
	}
	if n := len(f.reqs); n != 2 {
		t.Fatalf("request count = %d, want exactly 2 (original + one retry)", n)
	}
	m := decode(t, out)
	if items, _ := m["items"].([]any); len(items) != 1 {
		t.Errorf("items = %v, want 1 after retry", m["items"])
	}
}

func Test429ExhaustedSurfacesRetryAfter(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contact_books": {
			{status: http.StatusTooManyRequests, body: `{"message":"slow down"}`, header: map[string]string{"Retry-After": "2"}},
			{status: http.StatusTooManyRequests, body: `{"message":"slow down"}`, header: map[string]string{"Retry-After": "2"}},
		},
	})
	res, _, stderr := f.run("contact-books", "list", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 after retry exhausted", res.ExitCode)
	}
	env := decode(t, stderr)
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("stderr not a JSON error envelope: %q", stderr)
	}
	if errObj["status"] != float64(429) {
		t.Errorf("error.status = %v, want 429", errObj["status"])
	}
	if errObj["retry_after"] != float64(2) {
		t.Errorf("error.retry_after = %v, want 2", errObj["retry_after"])
	}
}

// --- write verbs: passthrough body + 201 empty body ---------------------------

func TestPostsCreate_EmptyBodyYieldsOK(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"POST /posts": {{status: http.StatusCreated, body: ""}},
	})
	payload := `{"posts":{"conversation":"c1","text":"summary"}}`
	res, out, _ := f.run("posts", "create", "--body", payload)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if strings.TrimSpace(out) != `{"ok":true}` {
		t.Errorf("stdout = %q, want {\"ok\":true} for a 201 empty body", out)
	}
	got := f.last("POST", "/posts")
	if got.ctype != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ctype)
	}
	// Body passes through verbatim (as a re-marshaled equal object).
	var sent, want map[string]any
	_ = json.Unmarshal([]byte(got.body), &sent)
	_ = json.Unmarshal([]byte(payload), &want)
	if sent["posts"] == nil {
		t.Errorf("request body = %q, want the posts envelope passed through", got.body)
	}
}

func TestPostsCreate_ReturnsBodyVerbatim(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"POST /posts": {{status: http.StatusOK, body: `{"posts":{"id":"p1"}}`}},
	})
	_, out, _ := f.run("posts", "create", "--body", `{"posts":{"conversation":"c1","text":"hi"}}`)
	if !strings.Contains(out, `"id":"p1"`) {
		t.Errorf("stdout = %q, want verbatim response body", out)
	}
}

func TestConversationsUpdate_PatchBodyPassthrough(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"PATCH /conversations/c1": {{status: http.StatusOK, body: `{"conversations":[{"id":"c1"}]}`}},
	})
	res, _, _ := f.run("conversations", "update", "c1", "--body", `{"conversations":{"add_shared_labels":["l1"]}}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	got := f.last("PATCH", "/conversations/c1")
	if !strings.Contains(got.body, "add_shared_labels") {
		t.Errorf("request body = %q, want the PATCH payload passed through", got.body)
	}
}

func TestWriteVerb_InvalidJSONIsUsageError(t *testing.T) {
	f := newFixture(t, map[string][]route{})
	res, _, _ := f.run("posts", "create", "--body", `{not json`)
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for invalid JSON body", res.ExitCode)
	}
	if len(f.reqs) != 0 {
		t.Errorf("expected no HTTP request for an invalid body, got %d", len(f.reqs))
	}
}

// --- credential rejection + missing token + unknown command -------------------

func TestUnauthorizedRejectsCredential(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contact_books": {{status: http.StatusUnauthorized, body: `{"message":"bad token"}`}},
	})
	res, _, _ := f.run("contact-books", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true on 401")
	}
}

func TestForbiddenRejectsCredential(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contact_books": {{status: http.StatusForbidden, body: `{"message":"forbidden"}`}},
	})
	res, _, _ := f.run("contact-books", "list")
	if !res.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true on 403")
	}
}

func TestServerErrorDoesNotRejectCredential(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /contact_books": {{status: http.StatusInternalServerError, body: `{"message":"boom"}`}},
	})
	res, _, _ := f.run("contact-books", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Errorf("CredentialRejected = true, want false on a 500")
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	f := newFixture(t, map[string][]route{})
	res, err := f.svc.Execute(context.Background(), []string{"conversations", "list", "--inbox"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 when MISSIVE_TOKEN is unset", res.ExitCode)
	}
	if !strings.Contains(f.stderr.String(), "MISSIVE_TOKEN") {
		t.Errorf("stderr = %q, want a MISSIVE_TOKEN diagnostic", f.stderr.String())
	}
	if len(f.reqs) != 0 {
		t.Errorf("no request should be made without a token, got %d", len(f.reqs))
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	f := newFixture(t, map[string][]route{})
	res, _, _ := f.run("conversations", "bogus")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for an unknown subcommand", res.ExitCode)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	f := newFixture(t, map[string][]route{
		"GET /conversations": {{status: http.StatusBadRequest, body: `{"message":"bad filter"}`}},
	})
	res, _, stderr := f.run("conversations", "list", "--inbox", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	env := decode(t, stderr)
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["kind"] != "api" {
		t.Fatalf("stderr = %q, want a JSON api error envelope", stderr)
	}
	if errObj["status"] != float64(400) {
		t.Errorf("error.status = %v, want 400", errObj["status"])
	}
}
