package mixpanel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Mixpanel server saw.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Query       url.Values
	Body        []byte
}

// fakeServer answers every request with status+response, recording the last
// request into got.
func fakeServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return fakeServerWithHeaders(t, status, response, nil, got)
}

func fakeServerWithHeaders(t *testing.T, status int, response string, headers map[string]string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        body,
		}
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

const (
	testUser    = "svc.abc123.mp-service-account"
	testSecret  = "s3cr3t"
	testProject = "3193719"
)

func wantAuthHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(testUser+":"+testSecret))
}

// packedCreds builds the MIXPANEL_CREDENTIALS JSON payload the token gateway
// injects (single-secret storage). region "" omits the field (service defaults
// to us).
func packedCreds(user, secret, project, region string) string {
	m := map[string]string{"username": user, "secret": secret, "project_id": project}
	if region != "" {
		m["region"] = region
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// runOn executes the service against srv pointed at all three host families,
// with the default US region, returning the outcome.
func runOn(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	return runWithEnv(t, srv, map[string]string{
		EnvCredentials: packedCreds(testUser, testSecret, testProject, ""),
	}, args...)
}

func runWithEnv(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	if srv != nil {
		svc.HC = srv.Client()
		svc.QueryBaseURL = srv.URL
		svc.AppBaseURL = srv.URL
		svc.ExportBaseURL = srv.URL
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// --- Host resolution (region correctness proven without a live EU/India creds) ---

func TestResolveHosts_AllRegions(t *testing.T) {
	cases := []struct {
		region             string
		query, app, export string
	}{
		{"", "https://mixpanel.com/api/query", "https://mixpanel.com/api/app", "https://data.mixpanel.com/api/2.0"},
		{"us", "https://mixpanel.com/api/query", "https://mixpanel.com/api/app", "https://data.mixpanel.com/api/2.0"},
		{"US", "https://mixpanel.com/api/query", "https://mixpanel.com/api/app", "https://data.mixpanel.com/api/2.0"},
		{"eu", "https://eu.mixpanel.com/api/query", "https://eu.mixpanel.com/api/app", "https://data-eu.mixpanel.com/api/2.0"},
		{"in", "https://in.mixpanel.com/api/query", "https://in.mixpanel.com/api/app", "https://data-in.mixpanel.com/api/2.0"},
	}
	for _, tc := range cases {
		h, err := resolveHosts(tc.region)
		if err != nil {
			t.Fatalf("resolveHosts(%q) error: %v", tc.region, err)
		}
		if h.query != tc.query || h.app != tc.app || h.export != tc.export {
			t.Errorf("resolveHosts(%q) = %+v, want query=%s app=%s export=%s", tc.region, h, tc.query, tc.app, tc.export)
		}
	}
}

func TestResolveHosts_Invalid(t *testing.T) {
	if _, err := resolveHosts("mars"); err == nil {
		t.Fatal("resolveHosts(mars) = nil error, want invalid-region error")
	}
}

func TestInvalidRegion_ExitsConfigError(t *testing.T) {
	result, _, stderr := runWithEnv(t, nil, map[string]string{
		EnvCredentials: packedCreds(testUser, testSecret, testProject, "mars"),
	}, "me")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "invalid MIXPANEL_REGION") {
		t.Errorf("stderr = %q, want invalid-region message", stderr)
	}
}

// --- Missing / malformed credentials ---

func TestMissingCredential_ExitsOne(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"no payload", map[string]string{}, EnvCredentials + " is not set"},
		{"bad json", map[string]string{EnvCredentials: "not-json"}, "is not valid JSON"},
		{"no username", map[string]string{EnvCredentials: packedCreds("", testSecret, testProject, "")}, "username"},
		{"no secret", map[string]string{EnvCredentials: packedCreds(testUser, "", testProject, "")}, "secret"},
		{"no project", map[string]string{EnvCredentials: packedCreds(testUser, testSecret, "", "")}, "project_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := runWithEnv(t, nil, tc.env, "me")
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want mention of %q", stderr, tc.want)
			}
		})
	}
}

func TestMissingCredential_JSONEnvelope(t *testing.T) {
	result, _, stderr := runWithEnv(t, nil, map[string]string{}, "me", "--json")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	env := decodeEnvelope(t, stderr)
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

// --- Auth header + project_id injection ---

func TestSegmentation_AuthAndProjectAndParams(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{"data":{"series":[]}}`, &got)
	defer srv.Close()

	result, stdout, stderr := runOn(t, srv, "segmentation",
		"--event", "Signup", "--from", "2026-07-01", "--to", "2026-07-07",
		"--on", `properties["plan"]`, "--where", `properties["country"]=="US"`,
		"--type", "unique", "--unit", "day")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", result.ExitCode, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/segmentation" {
		t.Errorf("request = %s %s, want GET /segmentation", got.Method, got.Path)
	}
	if got.Auth != wantAuthHeader() {
		t.Errorf("Authorization = %q, want %q", got.Auth, wantAuthHeader())
	}
	if got.Query.Get("project_id") != testProject {
		t.Errorf("project_id = %q, want %q", got.Query.Get("project_id"), testProject)
	}
	assertQuery(t, got.Query, map[string]string{
		"event": "Signup", "from_date": "2026-07-01", "to_date": "2026-07-07",
		"on": `properties["plan"]`, "where": `properties["country"]=="US"`,
		"type": "unique", "unit": "day",
	})
	if strings.TrimSpace(stdout) != `{"data":{"series":[]}}` {
		t.Errorf("stdout = %q, want verbatim JSON passthrough", stdout)
	}
}

func TestSegmentation_MissingRequiredFlag_ExitsTwo(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	// --event omitted → cobra required-flag error → exit 2.
	result, _, _ := runOn(t, srv, "segmentation", "--from", "2026-07-01", "--to", "2026-07-07")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for missing required flag", result.ExitCode)
	}
}

// --- events / events-names ---

func TestEvents_MarshalsEventArray(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{"data":{}}`, &got)
	defer srv.Close()
	result, _, _ := runOn(t, srv, "events", "--event", "A", "--event", "B", "--type", "general", "--unit", "day", "--interval", "7")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	if got.Path != "/events" {
		t.Errorf("path = %q, want /events", got.Path)
	}
	if got.Query.Get("event") != `["A","B"]` {
		t.Errorf("event = %q, want JSON array [\"A\",\"B\"]", got.Query.Get("event"))
	}
}

func TestEventsNames_HitsNamesEndpoint(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `["Signup","Purchase"]`, &got)
	defer srv.Close()
	result, stdout, _ := runOn(t, srv, "events-names")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	if got.Path != "/events/names" {
		t.Errorf("path = %q, want /events/names", got.Path)
	}
	if got.Query.Get("project_id") != testProject {
		t.Errorf("project_id missing on events-names")
	}
	if strings.TrimSpace(stdout) != `["Signup","Purchase"]` {
		t.Errorf("stdout = %q", stdout)
	}
}

// --- funnels ---

func TestFunnelsList_And_Run(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	if r, _, _ := runOn(t, srv, "funnels", "list"); r.ExitCode != 0 || got.Path != "/funnels/list" {
		t.Errorf("funnels list: exit=%d path=%q", r.ExitCode, got.Path)
	}
	if r, _, _ := runOn(t, srv, "funnels", "run", "--funnel-id", "42", "--from", "2026-07-01", "--to", "2026-07-07"); r.ExitCode != 0 {
		t.Fatalf("funnels run exit=%d", r.ExitCode)
	}
	if got.Path != "/funnels" || got.Query.Get("funnel_id") != "42" {
		t.Errorf("funnels run: path=%q funnel_id=%q", got.Path, got.Query.Get("funnel_id"))
	}
}

// --- POST body endpoints ---

func TestEngage_PostsFormBodyWithProjectInQuery(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	result, _, stderr := runOn(t, srv, "engage",
		"--where", `properties["$country_code"]=="US"`,
		"--output-properties", "$email", "--output-properties", "$name",
		"--page", "2")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr=%q)", result.ExitCode, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/engage" {
		t.Errorf("request = %s %s, want POST /engage", got.Method, got.Path)
	}
	if !strings.HasPrefix(got.ContentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q, want form-urlencoded", got.ContentType)
	}
	// project_id stays in the query string even for POST.
	if got.Query.Get("project_id") != testProject {
		t.Errorf("project_id = %q, want in query string", got.Query.Get("project_id"))
	}
	form, err := url.ParseQuery(string(got.Body))
	if err != nil {
		t.Fatalf("body not form-encoded: %v", err)
	}
	if form.Get("where") != `properties["$country_code"]=="US"` {
		t.Errorf("where = %q", form.Get("where"))
	}
	if form.Get("output_properties") != `["$email","$name"]` {
		t.Errorf("output_properties = %q, want JSON array", form.Get("output_properties"))
	}
	if form.Get("page") != "2" {
		t.Errorf("page = %q, want 2", form.Get("page"))
	}
	// project_id must NOT be duplicated into the body.
	if form.Get("project_id") != "" {
		t.Errorf("project_id leaked into POST body: %q", form.Get("project_id"))
	}
}

func TestEngage_NonIntegerPage_ExitsTwo(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	result, _, _ := runOn(t, srv, "engage", "--page", "abc")
	if result.ExitCode != 2 {
		t.Errorf("exit = %d, want 2 for non-integer page", result.ExitCode)
	}
}

func TestCohortsList_IsPost(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()
	result, _, _ := runOn(t, srv, "cohorts", "list")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	if got.Method != http.MethodPost || got.Path != "/cohorts/list" {
		t.Errorf("request = %s %s, want POST /cohorts/list", got.Method, got.Path)
	}
	if got.Query.Get("project_id") != testProject {
		t.Errorf("project_id missing on cohorts/list query string")
	}
}

// --- insights / retention ---

func TestInsights_RequiresBookmark(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	if r, _, _ := runOn(t, srv, "insights"); r.ExitCode != 2 {
		t.Errorf("insights without --bookmark-id exit=%d, want 2", r.ExitCode)
	}
	if r, _, _ := runOn(t, srv, "insights", "--bookmark-id", "99"); r.ExitCode != 0 || got.Query.Get("bookmark_id") != "99" {
		t.Errorf("insights: exit=%d bookmark_id=%q", r.ExitCode, got.Query.Get("bookmark_id"))
	}
}

func TestRetention_HitsEndpoint(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	runOn(t, srv, "retention", "--from", "2026-07-01", "--to", "2026-07-07", "--born-event", "Signup", "--event", "Login")
	if got.Path != "/retention" || got.Query.Get("born_event") != "Signup" {
		t.Errorf("retention: path=%q born_event=%q", got.Path, got.Query.Get("born_event"))
	}
	runOn(t, srv, "retention-frequency", "--from", "2026-07-01", "--to", "2026-07-07")
	if got.Path != "/retention/addiction" {
		t.Errorf("retention-frequency path=%q, want /retention/addiction", got.Path)
	}
}

// --- lexicon path + me app host ---

func TestLexiconList_PathCarriesProjectID(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()
	result, _, _ := runOn(t, srv, "lexicon", "list")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	if got.Path != "/projects/"+testProject+"/schemas" {
		t.Errorf("path = %q, want /projects/%s/schemas", got.Path, testProject)
	}
}

func TestMe_HitsAppMe(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, `{"results":{"id":1}}`, &got)
	defer srv.Close()
	result, _, _ := runOn(t, srv, "me")
	if result.ExitCode != 0 || got.Path != "/me" {
		t.Errorf("me: exit=%d path=%q", result.ExitCode, got.Path)
	}
	if got.Auth != wantAuthHeader() {
		t.Errorf("me auth header wrong: %q", got.Auth)
	}
}

// --- export: JSONL passthrough on the export host ---

func TestExport_StreamsJSONLAndRequiresWindow(t *testing.T) {
	jsonl := `{"event":"A","properties":{}}` + "\n" + `{"event":"B","properties":{}}` + "\n"
	var got capturedRequest
	srv := fakeServer(t, http.StatusOK, jsonl, &got)
	defer srv.Close()

	// missing --from/--to → exit 2
	if r, _, _ := runOn(t, srv, "export", "--from", "2026-07-01"); r.ExitCode != 2 {
		t.Errorf("export missing --to exit=%d, want 2", r.ExitCode)
	}

	result, stdout, _ := runOn(t, srv, "export", "--from", "2026-07-01", "--to", "2026-07-02", "--event", "A")
	if result.ExitCode != 0 {
		t.Fatalf("export exit=%d", result.ExitCode)
	}
	if got.Path != "/export" {
		t.Errorf("path = %q, want /export", got.Path)
	}
	if got.Query.Get("project_id") != testProject || got.Query.Get("from_date") != "2026-07-01" {
		t.Errorf("export query = %v", got.Query)
	}
	if got.Query.Get("event") != `["A"]` {
		t.Errorf("export event = %q, want JSON array", got.Query.Get("event"))
	}
	if stdout != jsonl {
		t.Errorf("stdout = %q, want verbatim JSONL passthrough", stdout)
	}
}

// --- Error envelopes: credential (401/403), rateLimit (429), generic (5xx) ---

func TestError_401_CredentialKind(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusUnauthorized, `{"error":"invalid credentials"}`, &got)
	defer srv.Close()
	result, _, stderr := runOn(t, srv, "me", "--json")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true for 401")
	}
	env := decodeEnvelope(t, stderr)
	if env.Error.Kind != "credential" || env.Error.Status != 401 {
		t.Errorf("envelope = %+v, want kind=credential status=401", env.Error)
	}
}

func TestError_403_CredentialKind(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusForbidden, `{"error":"forbidden"}`, &got)
	defer srv.Close()
	result, _, stderr := runOn(t, srv, "segmentation", "--event", "X", "--from", "2026-07-01", "--to", "2026-07-02", "--json")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 + credential rejected", result)
	}
	env := decodeEnvelope(t, stderr)
	if env.Error.Kind != "credential" {
		t.Errorf("kind = %q, want credential", env.Error.Kind)
	}
}

func TestError_429_RateLimitKindWithRetryAfter(t *testing.T) {
	var got capturedRequest
	srv := fakeServerWithHeaders(t, http.StatusTooManyRequests, `{"error":"too many requests"}`,
		map[string]string{"Retry-After": "37"}, &got)
	defer srv.Close()
	result, _, stderr := runOn(t, srv, "segmentation", "--event", "X", "--from", "2026-07-01", "--to", "2026-07-02", "--json")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	// A rate limit is transient — NOT a credential rejection.
	if result.CredentialRejected {
		t.Error("CredentialRejected = true, want false for 429 (transient)")
	}
	env := decodeEnvelope(t, stderr)
	if env.Error.Kind != "rateLimit" || env.Error.Status != 429 {
		t.Errorf("envelope = %+v, want kind=rateLimit status=429", env.Error)
	}
	if env.Error.RetryAfterSeconds != 37 {
		t.Errorf("retry_after_seconds = %d, want 37", env.Error.RetryAfterSeconds)
	}
}

func TestError_500_GenericApiKind(t *testing.T) {
	var got capturedRequest
	srv := fakeServer(t, http.StatusInternalServerError, `boom`, &got)
	defer srv.Close()
	result, _, stderr := runOn(t, srv, "me", "--json")
	if result.ExitCode != 1 || result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 without credential rejection", result)
	}
	env := decodeEnvelope(t, stderr)
	if env.Error.Kind != "api" || env.Error.Status != 500 {
		t.Errorf("envelope = %+v, want kind=api status=500", env.Error)
	}
}

// --- helpers ---

type errorEnvelope struct {
	Error struct {
		Message           string `json:"message"`
		Kind              string `json:"kind"`
		Status            int    `json:"status"`
		RetryAfterSeconds int    `json:"retry_after_seconds"`
	} `json:"error"`
}

func decodeEnvelope(t *testing.T, stderr string) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%q)", err, stderr)
	}
	return env
}

func assertQuery(t *testing.T, q url.Values, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("query[%s] = %q, want %q", k, q.Get(k), v)
		}
	}
}
