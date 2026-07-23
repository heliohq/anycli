package zuora

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// run drives the service against a fake server and returns stdout, stderr, and
// the execution result. BaseURL is the httptest override; env still carries the
// id/secret pair the way the credential binding injects them.
func run(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (string, string, executionResult) {
	t.Helper()
	var out, errOut bytes.Buffer
	s := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errOut}
	res, err := s.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error (should be nil): %v", err)
	}
	return out.String(), errOut.String(), executionResult{ExitCode: res.ExitCode, CredentialRejected: res.CredentialRejected}
}

type executionResult struct {
	ExitCode           int
	CredentialRejected bool
}

// liveEnv is the credential set an unspecified base URL leaves to the BaseURL
// override; only id/secret matter to the service once BaseURL is set.
func liveEnv() map[string]string {
	return map[string]string{EnvClientID: "cid", EnvClientSecret: "sec"}
}

func TestResolveBaseURL(t *testing.T) {
	s := &Service{}
	if got := s.resolveBaseURL("https://rest.apisandbox.zuora.com"); got != "https://rest.apisandbox.zuora.com" {
		t.Errorf("env base url = %q, want the env value verbatim", got)
	}
	if got := s.resolveBaseURL("  https://rest.eu.zuora.com  "); got != "https://rest.eu.zuora.com" {
		t.Errorf("env base url = %q, want trimmed", got)
	}
	override := &Service{BaseURL: "https://example.test"}
	if got := override.resolveBaseURL("https://rest.zuora.com"); got != "https://example.test" {
		t.Errorf("BaseURL override = %q, want the override", got)
	}
}

func TestMissingCredentials(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"no base url", map[string]string{EnvClientID: "cid", EnvClientSecret: "sec"}},
		{"no client id", map[string]string{EnvBaseURL: "https://rest.zuora.com", EnvClientSecret: "sec"}},
		{"no secret", map[string]string{EnvBaseURL: "https://rest.zuora.com", EnvClientID: "cid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			s := &Service{Out: &out, Err: &errOut}
			res, err := s.Execute(context.Background(), []string{"catalog", "products"}, tc.env)
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if res.ExitCode != 1 {
				t.Errorf("exit = %d, want 1", res.ExitCode)
			}
			if !strings.Contains(errOut.String(), "ZUORA_BASE_URL") {
				t.Errorf("stderr = %q, want mention of the required env vars", errOut.String())
			}
		})
	}
}

// TestClientCredentialsExchange asserts the token POST is form-encoded with the
// grant + id/secret in the BODY and carries NO Authorization header (Zuora's
// createToken contract), and the minted bearer is injected on the data call.
func TestClientCredentialsExchange(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/catalog/products": {200, `{"success":true,"products":[{"id":"P1"}]}`},
	})
	defer srv.Close()

	out, errOut, res := run(t, srv, liveEnv(), "catalog", "products")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr %q)", res.ExitCode, errOut)
	}
	tok := findReq(reqs, "POST", "/oauth/token")
	if tok == nil {
		t.Fatal("no token exchange request was made")
	}
	if got := tok.Form.Get("grant_type"); got != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", got)
	}
	if got := tok.Form.Get("client_id"); got != "cid" {
		t.Errorf("client_id form field = %q, want cid", got)
	}
	if got := tok.Form.Get("client_secret"); got != "sec" {
		t.Errorf("client_secret form field = %q, want sec", got)
	}
	if tok.Auth != "" {
		t.Errorf("token request carried Authorization %q, want none (Zuora forbids auth headers on /oauth/token)", tok.Auth)
	}
	if !strings.HasPrefix(tok.ContentType, "application/x-www-form-urlencoded") {
		t.Errorf("token Content-Type = %q, want form-urlencoded", tok.ContentType)
	}
	data := findReq(reqs, "GET", "/v1/catalog/products")
	if data == nil {
		t.Fatal("no data request was made")
	}
	if data.Auth != "Bearer A-TEST-BEARER" {
		t.Errorf("data Authorization = %q, want the minted bearer", data.Auth)
	}
	if !strings.Contains(out, "\"products\"") {
		t.Errorf("stdout = %q, want the passthrough products body", out)
	}
}

// TestSingleTokenMintPerInvocation proves the bearer is minted once and reused
// across every REST call in one command invocation (Zuora rate-limits minting).
func TestSingleTokenMintPerInvocation(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/accounts/A-1":         {200, `{"success":true,"id":"A-1"}`},
		"GET /v1/accounts/A-1/summary": {200, `{"success":true,"basicInfo":{"id":"A-1"}}`},
	})
	defer srv.Close()

	// Two data calls in one process would each mint if the cache were broken —
	// but a single Execute only runs one subcommand, so drive two subcommands
	// against the SAME service instance is not how anycli runs. Instead assert
	// that within one command that makes a single call, exactly one mint occurs,
	// and separately that the client caches (unit-level) below.
	_, errOut, res := run(t, srv, liveEnv(), "account", "get", "A-1")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr %q)", res.ExitCode, errOut)
	}
	if n := countReq(reqs, "POST", "/oauth/token"); n != 1 {
		t.Errorf("token mints = %d, want exactly 1", n)
	}
}

// TestClientCachesBearer proves the client mints once and reuses the cached
// bearer across multiple call()s (the per-invocation reuse contract).
func TestClientCachesBearer(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/accounts/A-1":         {200, `{"success":true,"id":"A-1"}`},
		"GET /v1/accounts/A-1/summary": {200, `{"success":true}`},
	})
	defer srv.Close()

	cl := &client{baseURL: srv.URL, clientID: "cid", secret: "sec", hc: srv.Client()}
	if _, err := cl.call(context.Background(), "GET", "/v1/accounts/A-1", nil, nil); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := cl.call(context.Background(), "GET", "/v1/accounts/A-1/summary", nil, nil); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if n := countReq(reqs, "POST", "/oauth/token"); n != 1 {
		t.Errorf("token mints across two calls = %d, want exactly 1 (cached)", n)
	}
}

func TestAccountSummaryPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/accounts/A-1/summary": {200, `{"success":true,"basicInfo":{"id":"A-1"}}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "account", "summary", "A-1")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr %q)", res.ExitCode, errOut)
	}
	if findReq(reqs, "GET", "/v1/accounts/A-1/summary") == nil {
		t.Error("summary did not GET /v1/accounts/A-1/summary")
	}
}

func TestSubscriptionListPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/subscriptions/accounts/A-1": {200, `{"success":true,"subscriptions":[]}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "subscription", "list", "A-1")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr %q)", res.ExitCode, errOut)
	}
	if findReq(reqs, "GET", "/v1/subscriptions/accounts/A-1") == nil {
		t.Error("subscription list did not GET the by-account path")
	}
}

// TestInvoiceListUsesZOQL proves invoice list posts a bound-literal ZOQL query
// to /v1/action/query rather than a nonexistent list-by-account GET.
func TestInvoiceListUsesZOQL(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v1/action/query": {200, `{"done":true,"size":0,"records":[]}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "invoice", "list", "A-1", "--limit", "5")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr %q)", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/v1/action/query")
	if req == nil {
		t.Fatal("invoice list did not POST /v1/action/query")
	}
	var payload struct {
		QueryString string `json:"queryString"`
	}
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		t.Fatalf("query payload not JSON: %v", err)
	}
	if !strings.Contains(payload.QueryString, "from Invoice where AccountId = 'A-1'") {
		t.Errorf("ZOQL = %q, want a bound AccountId literal over Invoice", payload.QueryString)
	}
	if !strings.Contains(payload.QueryString, "limit 5") {
		t.Errorf("ZOQL = %q, want the limit applied", payload.QueryString)
	}
}

// TestZOQLLiteralEscaping proves an account key with a single quote is escaped
// (doubled), not interpolated raw into the where-clause.
func TestZOQLLiteralEscaping(t *testing.T) {
	if got := zoqlLiteral("A'1"); got != "'A''1'" {
		t.Errorf("zoqlLiteral = %q, want doubled-quote escaping", got)
	}
}

func TestQueryRequiresSelect(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	// Missing --zoql is a usage error (exit 2), no request sent.
	_, _, res := run(t, srv, liveEnv(), "query")
	if res.ExitCode != 2 {
		t.Errorf("empty query exit = %d, want 2", res.ExitCode)
	}
	// A non-SELECT statement is refused locally (read-only guard), exit 2.
	_, _, res = run(t, srv, liveEnv(), "query", "--zoql", "update Account set Name = 'x'")
	if res.ExitCode != 2 {
		t.Errorf("non-select query exit = %d, want 2", res.ExitCode)
	}
	if countReq(reqs, "POST", "/v1/action/query") != 0 {
		t.Error("a refused query still hit the network")
	}
}

func TestQuerySendsZOQL(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v1/action/query": {200, `{"done":true,"size":1,"records":[{"Id":"a1"}]}`},
	})
	defer srv.Close()
	out, errOut, res := run(t, srv, liveEnv(), "query", "--zoql", "select Id from Account")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr %q)", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/v1/action/query")
	if req == nil {
		t.Fatal("query did not POST /v1/action/query")
	}
	if !strings.Contains(out, "\"records\"") {
		t.Errorf("stdout = %q, want the passthrough query body", out)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	_, _, res := run(t, srv, liveEnv(), "account", "nope")
	if res.ExitCode != 2 {
		t.Errorf("unknown subcommand exit = %d, want 2", res.ExitCode)
	}
}

// TestErrorEnvelopes covers the three distinct Zuora failure shapes plus the
// 200-with-error guard — each must be exit 1, and a 401 must set credential
// rejection.
func TestErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		body           string
		wantRejected   bool
		wantInStderr   string
		route          string
		args           []string
		expectMsgInErr bool
	}{
		{
			name:         "lowercase reasons on non-2xx",
			status:       400,
			body:         `{"success":false,"processId":"P","reasons":[{"code":50000040,"message":"bad account"}]}`,
			wantInStderr: "bad account",
			route:        "GET /v1/accounts/A-1",
			args:         []string{"account", "get", "A-1"},
		},
		{
			name:         "capitalized Errors on Action query (non-2xx)",
			status:       400,
			body:         `{"Success":false,"Errors":[{"Code":"INVALID_FIELD","Message":"unknown field"}]}`,
			wantInStderr: "unknown field",
			route:        "POST /v1/action/query",
			args:         []string{"query", "--zoql", "select Bogus from Account"},
		},
		{
			name:         "bare message on query 400",
			status:       400,
			body:         `{"message":"Unknown column 'Bogus'"}`,
			wantInStderr: "Unknown column",
			route:        "POST /v1/action/query",
			args:         []string{"query", "--zoql", "select Bogus from Account"},
		},
		{
			name:         "200 body with success:false is still a failure",
			status:       200,
			body:         `{"success":false,"reasons":[{"code":58730020,"message":"soft failure"}]}`,
			wantInStderr: "soft failure",
			route:        "GET /v1/accounts/A-1",
			args:         []string{"account", "get", "A-1"},
		},
		{
			name:         "401 is a credential rejection",
			status:       401,
			body:         `{"message":"Unauthorized"}`,
			wantRejected: true,
			wantInStderr: "Unauthorized",
			route:        "GET /v1/accounts/A-1",
			args:         []string{"account", "get", "A-1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{tc.route: {tc.status, tc.body}})
			defer srv.Close()
			_, errOut, res := run(t, srv, liveEnv(), tc.args...)
			if res.ExitCode != 1 {
				t.Errorf("exit = %d, want 1 (stderr %q)", res.ExitCode, errOut)
			}
			if res.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %v, want %v", res.CredentialRejected, tc.wantRejected)
			}
			if !strings.Contains(errOut, tc.wantInStderr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tc.wantInStderr)
			}
		})
	}
}

// TestJSONErrorEnvelope proves --json renders the structured api-error shape
// with the parsed status and Zuora code.
func TestJSONErrorEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/accounts/A-1": {404, `{"success":false,"reasons":[{"code":50000040,"message":"account not found"}]}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "account", "get", "A-1", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, errOut)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != 404 {
		t.Errorf("status = %d, want 404", env.Error.Status)
	}
	if env.Error.Code != "50000040" {
		t.Errorf("code = %q, want the Zuora numeric code", env.Error.Code)
	}
}

// TestFailureFromBody unit-tests the three-form detector directly.
func TestFailureFromBody(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantFail bool
		wantCode string
	}{
		{"lowercase reasons", `{"success":false,"reasons":[{"code":123,"message":"x"}]}`, true, "123"},
		{"capitalized Errors", `{"Success":false,"Errors":[{"Code":"E1","Message":"y"}]}`, true, "E1"},
		{"success true passes", `{"success":true,"records":[]}`, false, ""},
		{"no envelope passes", `{"products":[]}`, false, ""},
		{"explicit false no list", `{"success":false,"message":"m"}`, true, ""},
		{"not json passes", `<html>`, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failed, code, _ := failureFromBody([]byte(tc.body))
			if failed != tc.wantFail {
				t.Errorf("failed = %v, want %v", failed, tc.wantFail)
			}
			if failed && code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
		})
	}
}

// TestTokenExchangeRejection proves a 401 on the token mint itself is a
// credential rejection with exit 1.
func TestTokenExchangeRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /oauth/token": {401, `{"message":"invalid client"}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "catalog", "products")
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1 (stderr %q)", res.ExitCode, errOut)
	}
	if !res.CredentialRejected {
		t.Error("a 401 on token exchange must flag credential rejection")
	}
}
