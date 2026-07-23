package activecampaign

import (
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

// capturedRequest records one request the fake ActiveCampaign server saw.
type capturedRequest struct {
	Method      string
	Path        string
	Token       string
	ContentType string
	Query       url.Values
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newServer is a fake ActiveCampaign API. It records every request and answers
// from routes keyed by "METHOD /path"; an unmatched route returns 404 so a
// wrong path is a loud failure, not a silent pass.
func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Token:       r.Header.Get("Api-Token"),
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
		_, _ = w.Write([]byte(`{"message":"No Result found for Subscriber with id"}`))
	}))
}

// run executes one activecampaign invocation against srv with a valid token,
// capturing stdout/stderr.
func run(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf strings.Builder
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	env := map[string]string{EnvToken: "test-token", EnvURL: srv.URL}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res, out.String(), errBuf.String()
}

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

func bodyMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (%s)", err, b)
	}
	return m
}

func TestContactListInjectsTokenAndResolvesAPIPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/3/contacts": {status: 200, body: `{"contacts":[],"meta":{"total":"0"}}`},
	})
	defer srv.Close()

	res, out, _ := run(t, srv, "contact", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, http.MethodGet, "/api/3/contacts")
	if req == nil {
		t.Fatalf("no GET /api/3/contacts request; got %+v", reqs)
	}
	if req.Token != "test-token" {
		t.Errorf("Api-Token = %q, want test-token", req.Token)
	}
	if !strings.Contains(out, `"contacts"`) {
		t.Errorf("stdout did not passthrough provider body: %q", out)
	}
}

func TestBaseURLNormalizationVariants(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/3/tags": {status: 200, body: `{"tags":[]}`},
	})
	defer srv.Close()

	// Every accepted paste shape must resolve to <host>/api/3/tags. (Bare-host
	// scheme defaulting is covered by the pure TestNormalizeBaseURL below; it
	// forces https and so cannot ride the plain-http test server here.)
	variants := []string{
		srv.URL,             // canonical scheme+host
		srv.URL + "/",       // trailing slash
		srv.URL + "/api/3",  // stray /api/3
		srv.URL + "/api/3/", // stray /api/3 with slash
	}
	for _, v := range variants {
		reqs = nil
		var out, errBuf strings.Builder
		svc := &Service{HC: srv.Client(), Out: &out, Err: &errBuf}
		// BaseURL empty so normalization runs on the env-provided URL.
		env := map[string]string{EnvToken: "t", EnvURL: v}
		res, err := svc.Execute(context.Background(), []string{"tag", "list"}, env)
		if err != nil {
			t.Fatalf("variant %q transport error: %v", v, err)
		}
		if res.ExitCode != 0 {
			t.Fatalf("variant %q exit = %d (%s)", v, res.ExitCode, errBuf.String())
		}
		if findReq(reqs, http.MethodGet, "/api/3/tags") == nil {
			t.Errorf("variant %q did not resolve to /api/3/tags; got %+v", v, reqs)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://acct.api-us1.com", "https://acct.api-us1.com/api/3"},
		{"https://acct.api-us1.com/", "https://acct.api-us1.com/api/3"},
		{"https://acct.api-us1.com/api/3", "https://acct.api-us1.com/api/3"},
		{"https://acct.api-us1.com/api/3/", "https://acct.api-us1.com/api/3"},
		{"  https://acct.api-us1.com  ", "https://acct.api-us1.com/api/3"},
		{"acct.api-us1.com", "https://acct.api-us1.com/api/3"}, // bare host defaults to https
		{"http://local.test:8080", "http://local.test:8080/api/3"},
	}
	for _, c := range cases {
		got, err := normalizeBaseURL(c.in)
		if err != nil {
			t.Errorf("normalizeBaseURL(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := normalizeBaseURL("   "); err == nil {
		t.Error("empty input should error")
	}
	if _, err := normalizeBaseURL("https:///nohost"); err == nil {
		t.Error("missing host should error")
	}
}

func TestContactGetPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/3/contacts/42": {status: 200, body: `{"contact":{"id":"42"}}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "get", "42")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	if findReq(reqs, http.MethodGet, "/api/3/contacts/42") == nil {
		t.Fatalf("no GET contacts/42; got %+v", reqs)
	}
}

func TestContactListPassesLimitOffsetAndQuery(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/3/contacts": {status: 200, body: `{"contacts":[]}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "list", "--limit", "5", "--offset", "10",
		"--query", "email=a@b.com", "--query", "filters[status]=1")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, http.MethodGet, "/api/3/contacts")
	if req == nil {
		t.Fatalf("no list request")
	}
	if req.Query.Get("limit") != "5" || req.Query.Get("offset") != "10" {
		t.Errorf("limit/offset not passed: %v", req.Query)
	}
	if req.Query.Get("email") != "a@b.com" {
		t.Errorf("email query not passed: %v", req.Query)
	}
	if req.Query.Get("filters[status]") != "1" {
		t.Errorf("filters passthrough failed: %v", req.Query)
	}
}

func TestContactCreateWrapsContactObject(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/3/contacts": {status: 201, body: `{"contact":{"id":"7"}}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "create",
		"--email", "jane@example.com", "--first-name", "Jane",
		"--data", `{"phone":"+15551234"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, http.MethodPost, "/api/3/contacts")
	if req == nil {
		t.Fatalf("no create request")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	m := bodyMap(t, req.Body)
	contact, ok := m["contact"].(map[string]any)
	if !ok {
		t.Fatalf("body not wrapped under contact: %s", req.Body)
	}
	if contact["email"] != "jane@example.com" {
		t.Errorf("email = %v", contact["email"])
	}
	if contact["firstName"] != "Jane" {
		t.Errorf("firstName = %v", contact["firstName"])
	}
	if contact["phone"] != "+15551234" {
		t.Errorf("--data field not merged: %v", contact["phone"])
	}
}

func TestContactTagWrapsContactTag(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/3/contactTags": {status: 201, body: `{"contactTag":{"id":"1"}}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "tag", "--contact", "7", "--tag", "3")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, http.MethodPost, "/api/3/contactTags")
	if req == nil {
		t.Fatalf("no contactTags request")
	}
	inner, ok := bodyMap(t, req.Body)["contactTag"].(map[string]any)
	if !ok {
		t.Fatalf("body not wrapped under contactTag: %s", req.Body)
	}
	if inner["contact"] != "7" || inner["tag"] != "3" {
		t.Errorf("contactTag body = %v", inner)
	}
}

func TestContactSubscribeWrapsContactList(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/3/contactLists": {status: 201, body: `{"contactList":{"id":"1"}}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "subscribe", "--list", "2", "--contact", "7", "--status", "1")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, http.MethodPost, "/api/3/contactLists")
	if req == nil {
		t.Fatalf("no contactLists request")
	}
	inner := bodyMap(t, req.Body)["contactList"].(map[string]any)
	if inner["list"] != "2" || inner["contact"] != "7" || inner["status"] != "1" {
		t.Errorf("contactList body = %v", inner)
	}
}

func TestContactAutomateWrapsContactAutomation(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/3/contactAutomations": {status: 201, body: `{"contactAutomation":{"id":"1"}}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "automate", "--contact", "7", "--automation", "9")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, http.MethodPost, "/api/3/contactAutomations")
	if req == nil {
		t.Fatalf("no contactAutomations request")
	}
	inner := bodyMap(t, req.Body)["contactAutomation"].(map[string]any)
	if inner["contact"] != "7" || inner["automation"] != "9" {
		t.Errorf("contactAutomation body = %v", inner)
	}
}

func TestTagCreateWrapsTag(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/3/tags": {status: 201, body: `{"tag":{"id":"1"}}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "tag", "create", "--name", "vip", "--type", "contact", "--description", "big spenders")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, http.MethodPost, "/api/3/tags")
	inner := bodyMap(t, req.Body)["tag"].(map[string]any)
	if inner["tag"] != "vip" || inner["tagType"] != "contact" || inner["description"] != "big spenders" {
		t.Errorf("tag body = %v", inner)
	}
}

func TestDealCreateWrapsDeal(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/3/deals": {status: 201, body: `{"deal":{"id":"1"}}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "deal", "create", "--data", `{"title":"Big deal","value":"10000","currency":"usd"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, http.MethodPost, "/api/3/deals")
	inner := bodyMap(t, req.Body)["deal"].(map[string]any)
	if inner["title"] != "Big deal" || inner["value"] != "10000" {
		t.Errorf("deal body = %v", inner)
	}
}

func TestReadGroupsResolvePaths(t *testing.T) {
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"list", "list"}, "/api/3/lists"},
		{[]string{"list", "get", "5"}, "/api/3/lists/5"},
		{[]string{"tag", "list"}, "/api/3/tags"},
		{[]string{"deal", "list"}, "/api/3/deals"},
		{[]string{"deal", "get", "5"}, "/api/3/deals/5"},
		{[]string{"pipeline", "list"}, "/api/3/dealGroups"},
		{[]string{"stage", "list"}, "/api/3/dealStages"},
		{[]string{"campaign", "list"}, "/api/3/campaigns"},
		{[]string{"campaign", "get", "5"}, "/api/3/campaigns/5"},
		{[]string{"automation", "list"}, "/api/3/automations"},
		{[]string{"field", "list"}, "/api/3/fields"},
		{[]string{"account", "list"}, "/api/3/accounts"},
	}
	for _, c := range cases {
		var reqs []capturedRequest
		srv := newServer(t, &reqs, map[string]stub{
			"GET " + c.path: {status: 200, body: `{"ok":true}`},
		})
		res, _, errOut := run(t, srv, c.args...)
		if res.ExitCode != 0 {
			srv.Close()
			t.Fatalf("%v exit = %d (%s)", c.args, res.ExitCode, errOut)
		}
		if findReq(reqs, http.MethodGet, c.path) == nil {
			srv.Close()
			t.Fatalf("%v did not hit %s; got %+v", c.args, c.path, reqs)
		}
		srv.Close()
	}
}

func TestUnauthorizedIsCredentialRejectedExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/3/contacts": {status: 401, body: `{"message":"You do not have permission"}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Errorf("401 must classify as credential rejected")
	}
	if !strings.Contains(errOut, "401") {
		t.Errorf("stderr should mention the status: %q", errOut)
	}
}

func TestUnprocessableEntityJSONError(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/3/contacts": {status: 422, body: `{"errors":[{"title":"Email is not valid"}]}`},
	})
	defer srv.Close()

	res, _, errOut := run(t, srv, "contact", "create", "--email", "bad", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Errorf("422 must NOT be a credential rejection")
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, errOut)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("error envelope = %+v", env.Error)
	}
}

func TestMissingCredentialsExit1(t *testing.T) {
	svc := &Service{}
	// No token.
	res, err := svc.Execute(context.Background(), []string{"contact", "list"}, map[string]string{EnvURL: "https://x.api-us1.com"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("missing token exit = %d, want 1", res.ExitCode)
	}
	// No URL.
	res, _ = svc.Execute(context.Background(), []string{"contact", "list"}, map[string]string{EnvToken: "t"})
	if res.ExitCode != 1 {
		t.Errorf("missing url exit = %d, want 1", res.ExitCode)
	}
}

func TestUnknownSubcommandIsUsageExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()
	res, _, _ := run(t, srv, "contact", "frobnicate")
	if res.ExitCode != 2 {
		t.Errorf("unknown subcommand exit = %d, want 2", res.ExitCode)
	}
}
