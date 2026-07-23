package paypal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// run drives the service against a fake server and returns stdout, stderr, and
// the execution result.
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

func liveEnv() map[string]string {
	return map[string]string{EnvClientID: "cid", EnvClientSecret: "sec"}
}

func TestResolveBaseURL(t *testing.T) {
	s := &Service{}
	if got := s.resolveBaseURL("sandbox"); got != sandboxBaseURL {
		t.Errorf("sandbox host = %q, want %q", got, sandboxBaseURL)
	}
	if got := s.resolveBaseURL("live"); got != liveBaseURL {
		t.Errorf("live host = %q, want %q", got, liveBaseURL)
	}
	if got := s.resolveBaseURL(""); got != liveBaseURL {
		t.Errorf("empty env host = %q, want live %q", got, liveBaseURL)
	}
	override := &Service{BaseURL: "https://example.test"}
	if got := override.resolveBaseURL("sandbox"); got != "https://example.test" {
		t.Errorf("BaseURL override = %q, want the override", got)
	}
}

func TestMissingCredentials(t *testing.T) {
	var out, errOut bytes.Buffer
	s := &Service{Out: &out, Err: &errOut}
	res, err := s.Execute(context.Background(), []string{"invoice", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "PAYPAL_CLIENT_ID") {
		t.Errorf("stderr = %q, want mention of PAYPAL_CLIENT_ID", errOut.String())
	}
}

// TestClientCredentialsExchange asserts the token POST carries the
// client_credentials grant + correct Basic header, and the minted bearer is
// injected on the subsequent data call.
func TestClientCredentialsExchange(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/invoicing/invoices": {200, `{"items":[{"id":"INV2-1"}],"total_pages":1,"total_items":1}`},
	})
	defer srv.Close()

	out, errOut, res := run(t, srv, liveEnv(), "invoice", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (stderr %q)", res.ExitCode, errOut)
	}
	tok := findReq(reqs, "POST", "/v1/oauth2/token")
	if tok == nil {
		t.Fatal("no token exchange request was made")
	}
	if got := tok.Form.Get("grant_type"); got != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", got)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:sec"))
	if tok.Auth != wantAuth {
		t.Errorf("token Basic header = %q, want %q", tok.Auth, wantAuth)
	}
	data := findReq(reqs, "GET", "/v2/invoicing/invoices")
	if data == nil {
		t.Fatal("no data request was made")
	}
	if data.Auth != "Bearer A-TEST-BEARER" {
		t.Errorf("data Authorization = %q, want the minted bearer", data.Auth)
	}
	// list normalization: items -> results, page counters preserved, links dropped.
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, out)
	}
	if _, ok := env["results"]; !ok {
		t.Errorf("normalized output has no results key: %s", out)
	}
	if string(env["total_items"]) != "1" {
		t.Errorf("total_items = %s, want 1", env["total_items"])
	}
}

func TestHostSelection(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"sandbox", sandboxBaseURL},
		{"live", liveBaseURL},
		{"", liveBaseURL},
	}
	for _, tc := range cases {
		s := &Service{}
		if got := s.resolveBaseURL(tc.env); got != tc.want {
			t.Errorf("PAYPAL_ENV=%q -> %q, want %q", tc.env, got, tc.want)
		}
	}
}

func TestInvoiceListQueryParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/invoicing/invoices": {200, `{"items":[],"total_pages":0,"total_items":0}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "invoice", "list", "--page", "2", "--page-size", "50")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/v2/invoicing/invoices")
	if req.Query.Get("page") != "2" || req.Query.Get("page_size") != "50" || req.Query.Get("total_required") != "true" {
		t.Errorf("query = %v, want page=2 page_size=50 total_required=true", req.Query)
	}
}

func TestInvoiceSearchIsTopLevelSibling(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/invoicing/search-invoices": {200, `{"items":[{"id":"INV2-9"}],"total_items":1}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "invoice", "search", "--status", "SENT")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	if findReq(reqs, "POST", "/v2/invoicing/search-invoices") == nil {
		t.Error("search did not hit the top-level /v2/invoicing/search-invoices sibling")
	}
}

func TestTransactionWindowGuard(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	// 40-day window is over the 31-day maximum → usage error (exit 2), no call.
	_, errOut, res := run(t, srv, liveEnv(), "transaction", "list",
		"--start-date", "2026-06-01T00:00:00Z", "--end-date", "2026-07-11T00:00:00Z")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", res.ExitCode)
	}
	if !strings.Contains(errOut, "31-day") {
		t.Errorf("stderr = %q, want the 31-day guidance", errOut)
	}
	if findReq(reqs, "GET", "/v1/reporting/transactions") != nil {
		t.Error("an over-window request was sent; the guard should have blocked it")
	}
}

func TestTransactionListWithinWindow(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/reporting/transactions": {200, `{"transaction_details":[{"transaction_info":{"transaction_id":"T1"}}],"total_pages":1,"total_items":1}`},
	})
	defer srv.Close()
	out, errOut, res := run(t, srv, liveEnv(), "transaction", "list",
		"--start-date", "2026-07-01T00:00:00Z", "--end-date", "2026-07-15T00:00:00Z")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/v1/reporting/transactions")
	if req.Query.Get("start_date") != "2026-07-01T00:00:00Z" {
		t.Errorf("start_date query = %q", req.Query.Get("start_date"))
	}
	if !strings.Contains(out, "results") {
		t.Errorf("transaction_details was not normalized to results: %s", out)
	}
}

func TestCredentialRejectionOn401(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v1/oauth2/token": {401, `{"error":"invalid_client","error_description":"Client Authentication failed"}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "invoice", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (api)", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("a 401 token exchange was not classified as a credential rejection")
	}
	if !strings.Contains(errOut, "invalid_client") {
		t.Errorf("stderr = %q, want the provider error", errOut)
	}
}

func TestFeatureNotEnabled403(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/reporting/transactions": {403, `{"name":"NOT_AUTHORIZED","message":"Authorization failed due to insufficient permissions"}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, liveEnv(), "transaction", "list",
		"--start-date", "2026-07-01T00:00:00Z", "--end-date", "2026-07-10T00:00:00Z")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("a 403 feature-scope error must NOT be classified as a credential rejection")
	}
	if !strings.Contains(errOut, "Transaction Search") {
		t.Errorf("stderr = %q, want the feature-enablement hint", errOut)
	}
}

func TestValidationAndRateLimitStatusesRender(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"422", 422, `{"name":"UNPROCESSABLE_ENTITY","message":"bad shape","details":[{"issue":"INVALID_PARAMETER_SYNTAX","description":"x"}]}`},
		{"429", 429, `{"name":"RATE_LIMIT_REACHED","message":"too many requests"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				"GET /v2/invoicing/invoices": {tc.status, tc.body},
			})
			defer srv.Close()
			// plain render
			_, _, res := run(t, srv, liveEnv(), "invoice", "list")
			if res.ExitCode != 1 || res.CredentialRejected {
				t.Fatalf("exit=%d rejected=%v, want exit 1 not rejected", res.ExitCode, res.CredentialRejected)
			}
			// json render carries kind=api + status.
			_, errJSON, _ := run(t, srv, liveEnv(), "invoice", "list", "--json")
			var env struct {
				Error struct {
					Kind   string `json:"kind"`
					Status int    `json:"status"`
				} `json:"error"`
			}
			line := strings.TrimSpace(errJSON)
			if err := json.Unmarshal([]byte(line), &env); err != nil {
				t.Fatalf("json stderr not parseable: %v (%q)", err, line)
			}
			if env.Error.Kind != "api" || env.Error.Status != tc.status {
				t.Errorf("json error = %+v, want kind api status %d", env.Error, tc.status)
			}
		})
	}
}

func TestSubscriptionGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/billing/subscriptions/I-SUB": {200, `{"id":"I-SUB","status":"ACTIVE"}`},
	})
	defer srv.Close()
	out, errOut, res := run(t, srv, liveEnv(), "subscription", "get", "I-SUB")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	if !strings.Contains(out, `"status":"ACTIVE"`) {
		t.Errorf("object body was not emitted verbatim: %s", out)
	}
}

func TestBalanceList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v1/reporting/balances": {200, `{"balances":[{"currency":"USD","total_balance":{"value":"10.00"}}],"account_id":"ACC"}`},
	})
	defer srv.Close()
	out, errOut, res := run(t, srv, liveEnv(), "balance", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d (%s)", res.ExitCode, errOut)
	}
	if !strings.Contains(out, "results") {
		t.Errorf("balances not normalized to results: %s", out)
	}
}
