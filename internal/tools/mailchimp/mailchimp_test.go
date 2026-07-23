package mailchimp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records one request the fake Mailchimp server saw.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Query       url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newServer is a multi-route fake Mailchimp Marketing API: it answers each
// request from routes keyed by "METHOD /path" and records every request into
// reqs. An unmatched route returns a 404 problem-detail body.
func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        body,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"Resource Not Found","status":404,"detail":"The requested resource could not be found."}`))
	}))
}

// findReq returns the first recorded request matching method+path, or nil.
func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

// runTool executes one mailchimp command against s with the given token and
// returns stdout, stderr, and the execution result.
func runTool(t *testing.T, s *Service, token string, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf bytes.Buffer
	s.Out = &out
	s.Err = &errBuf
	env := map[string]string{}
	if token != "" {
		env[EnvAccessToken] = token
	}
	res, err := s.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected engine error: %v", err)
	}
	return out.String(), errBuf.String(), res
}

func TestPingPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /ping": {status: 200, body: `{"health_status":"Everything's Chimpy!"}`},
	})
	defer srv.Close()

	s := &Service{BaseURL: srv.URL}
	stdout, _, res := runTool(t, s, "test-token", "ping")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if stdout != `{"health_status":"Everything's Chimpy!"}`+"\n" {
		t.Errorf("stdout = %q, want provider JSON passthrough + newline", stdout)
	}
	req := findReq(reqs, "GET", "/ping")
	if req == nil {
		t.Fatal("no GET /ping recorded")
	}
	if req.Auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", req.Auth)
	}
}

func TestMissingToken(t *testing.T) {
	s := &Service{BaseURL: "http://unused.invalid"}
	_, stderr, res := runTool(t, s, "", "ping")
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(stderr, EnvAccessToken) {
		t.Errorf("stderr = %q, want mention of %s", stderr, EnvAccessToken)
	}
}

func TestResolveBaseAPIKeySuffix(t *testing.T) {
	// An API-key-shaped token resolves its dc from the key suffix; the
	// metadata endpoint must NOT be consulted.
	metaHits := 0
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metaHits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer meta.Close()

	s := &Service{MetadataURL: meta.URL}
	r := &requester{s: s, token: "anycli-fake-test-key-us6"}
	base, err := r.resolveBase(context.Background())
	if err != nil {
		t.Fatalf("resolveBase: %v", err)
	}
	if base != "https://us6.api.mailchimp.com/3.0" {
		t.Errorf("base = %q, want https://us6.api.mailchimp.com/3.0", base)
	}
	if metaHits != 0 {
		t.Errorf("metadata endpoint hit %d times, want 0 (api-key suffix wins)", metaHits)
	}
}

func TestResolveBaseOAuthMetadata(t *testing.T) {
	// An OAuth token (no dc suffix) resolves via GET /oauth2/metadata with the
	// documented `OAuth` auth scheme; api_endpoint is used verbatim + /3.0.
	var metaAuth string
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metaAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dc":"us6","accountname":"Freddie","api_endpoint":"https://us6.api.mailchimp.com"}`))
	}))
	defer meta.Close()

	s := &Service{MetadataURL: meta.URL}
	r := &requester{s: s, token: "oauthtoken123"}
	base, err := r.resolveBase(context.Background())
	if err != nil {
		t.Fatalf("resolveBase: %v", err)
	}
	if base != "https://us6.api.mailchimp.com/3.0" {
		t.Errorf("base = %q, want api_endpoint + /3.0", base)
	}
	if metaAuth != "OAuth oauthtoken123" {
		t.Errorf("metadata Authorization = %q, want OAuth scheme", metaAuth)
	}
}

func TestResolveBaseOAuthMetadataEndToEnd(t *testing.T) {
	// Full command run on the OAuth path: metadata's api_endpoint points at
	// the fake API server, so /3.0/lists must be hit there.
	var reqs []capturedRequest
	api := newServer(t, &reqs, map[string]stub{
		"GET /3.0/lists": {status: 200, body: `{"lists":[]}`},
	})
	defer api.Close()
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dc":"test","api_endpoint":"` + api.URL + `"}`))
	}))
	defer meta.Close()

	s := &Service{MetadataURL: meta.URL}
	stdout, _, res := runTool(t, s, "oauthtoken123", "audience", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if stdout != `{"lists":[]}`+"\n" {
		t.Errorf("stdout = %q", stdout)
	}
	req := findReq(reqs, "GET", "/3.0/lists")
	if req == nil {
		t.Fatal("no GET /3.0/lists recorded — api_endpoint not used verbatim")
	}
	if req.Auth != "Bearer oauthtoken123" {
		t.Errorf("API Authorization = %q, want Bearer", req.Auth)
	}
}

func TestResolveBaseMetadataRejected(t *testing.T) {
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer meta.Close()

	s := &Service{MetadataURL: meta.URL}
	r := &requester{s: s, token: "badtoken"}
	_, err := r.resolveBase(context.Background())
	if err == nil {
		t.Fatal("resolveBase: want error on 401 metadata")
	}
	if !execution.IsCredentialRejected(err) {
		t.Errorf("401 metadata error not marked credential-rejected: %v", err)
	}
}

func TestAPIErrorProblemDetail(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /lists": {status: 400, body: `{"type":"about:blank","title":"Invalid Resource","status":400,"detail":"The resource submitted could not be validated."}`},
	})
	defer srv.Close()

	s := &Service{BaseURL: srv.URL}
	_, stderr, res := runTool(t, s, "tok", "audience", "list")
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("400 must not reject the credential")
	}
	if !strings.Contains(stderr, "Invalid Resource") || !strings.Contains(stderr, "could not be validated") {
		t.Errorf("stderr = %q, want problem-detail title and detail", stderr)
	}
	if !strings.Contains(stderr, "HTTP 400") {
		t.Errorf("stderr = %q, want HTTP status", stderr)
	}
}

func TestAPIError401RejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /ping": {status: 401, body: `{"type":"about:blank","title":"API Key Invalid","status":401,"detail":"Your API key may be invalid."}`},
	})
	defer srv.Close()

	s := &Service{BaseURL: srv.URL}
	_, stderr, res := runTool(t, s, "tok", "ping")
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 must mark the credential rejected")
	}
	if !strings.Contains(stderr, "reconnect Mailchimp") {
		t.Errorf("stderr = %q, want a credential-rejected hint", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /lists": {status: 400, body: `{"title":"Invalid Resource","status":400,"detail":"nope"}`},
	})
	defer srv.Close()

	s := &Service{BaseURL: srv.URL}
	_, stderr, res := runTool(t, s, "tok", "audience", "list", "--json")
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %q (%v)", stderr, err)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != 400 {
		t.Errorf("envelope = %+v, want kind api status 400", envelope.Error)
	}
}

func TestUsageErrors(t *testing.T) {
	s := &Service{BaseURL: "http://unused.invalid"}

	// Unknown subcommand.
	_, _, res := runTool(t, s, "tok", "nonsense")
	if res.ExitCode != 2 {
		t.Errorf("unknown subcommand exit = %d, want 2", res.ExitCode)
	}

	// member get needs exactly one of --email / --hash.
	_, stderr, res := runTool(t, s, "tok", "member", "get", "L1")
	if res.ExitCode != 2 {
		t.Errorf("member get without selector exit = %d, want 2", res.ExitCode)
	}
	if !strings.Contains(stderr, "--email") {
		t.Errorf("stderr = %q, want mention of --email", stderr)
	}
	_, _, res = runTool(t, s, "tok", "member", "get", "L1", "--email", "a@b.c", "--hash", "deadbeef")
	if res.ExitCode != 2 {
		t.Errorf("member get with both selectors exit = %d, want 2", res.ExitCode)
	}

	// usage error under --json renders a kind:usage envelope.
	_, stderr, res = runTool(t, s, "tok", "member", "get", "L1", "--json")
	if res.ExitCode != 2 {
		t.Errorf("exit = %d, want 2", res.ExitCode)
	}
	var envelope struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %q (%v)", stderr, err)
	}
	if envelope.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", envelope.Error.Kind)
	}
}

func TestSubscriberHash(t *testing.T) {
	// Official docs example: MD5 of the lowercase address.
	got := subscriberHash("URIST.mcvankab@freddiesjokes.com")
	if got != "62eeb292278cc15f5817cb78f7790b08" {
		t.Errorf("subscriberHash = %q, want 62eeb292278cc15f5817cb78f7790b08", got)
	}
}

func TestListFlags(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /lists": {status: 200, body: `{"lists":[]}`},
	})
	defer srv.Close()

	s := &Service{BaseURL: srv.URL}
	_, _, res := runTool(t, s, "tok", "audience", "list", "--count", "5", "--offset", "10", "--fields", "lists.id,lists.name")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "GET", "/lists")
	if req == nil {
		t.Fatal("no GET /lists recorded")
	}
	if req.Query.Get("count") != "5" || req.Query.Get("offset") != "10" {
		t.Errorf("query = %v, want count=5 offset=10", req.Query)
	}
	if req.Query.Get("fields") != "lists.id,lists.name" {
		t.Errorf("fields = %q, want passthrough", req.Query.Get("fields"))
	}
}
