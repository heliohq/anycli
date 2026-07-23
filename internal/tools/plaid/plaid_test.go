package plaid

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records one request the fake Plaid server received.
type capturedRequest struct {
	Method      string
	Path        string
	ClientID    string
	Secret      string
	ContentType string
	Body        map[string]any
}

// newFake is a fake Plaid server: it answers each request from routes keyed by
// path and records every request into reqs. Unmatched routes return 404.
func newFake(t *testing.T, reqs *[]capturedRequest, routes map[string]struct {
	status int
	body   string
}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			ClientID:    r.Header.Get("PLAID-CLIENT-ID"),
			Secret:      r.Header.Get("PLAID-SECRET"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        m,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error_type":"INVALID_REQUEST","error_code":"NO_ROUTE","error_message":"not found","request_id":"req-404"}`))
	}))
}

type route = struct {
	status int
	body   string
}

// run executes the service against the fake server with sandbox creds, returning
// stdout, stderr, and the service Result.
func run(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (string, string, int, bool) {
	t.Helper()
	var out, errOut bytes.Buffer
	s := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errOut}
	res, err := s.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error (should be nil): %v", err)
	}
	return out.String(), errOut.String(), res.ExitCode, res.CredentialRejected
}

func sandboxEnv() map[string]string {
	return map[string]string{EnvClientID: "cid-123", EnvSecret: "sec-456", EnvEnvironment: "sandbox"}
}

// --- credential header + base-URL wiring -----------------------------------

func TestInstitutionsGet_SendsHeadersAndBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{
		"/institutions/get": {200, `{"institutions":[],"total":0,"request_id":"r1"}`},
	})
	defer srv.Close()

	out, errOut, code, _ := run(t, srv, sandboxEnv(),
		"institutions", "get", "--count", "5", "--offset", "2", "--country-codes", "US,CA")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, `"request_id":"r1"`) {
		t.Fatalf("stdout did not carry the provider body: %q", out)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/institutions/get" {
		t.Fatalf("unexpected request line: %s %s", r.Method, r.Path)
	}
	if r.ClientID != "cid-123" || r.Secret != "sec-456" {
		t.Fatalf("credentials not sent as headers: client=%q secret=%q", r.ClientID, r.Secret)
	}
	if r.ContentType != "application/json" {
		t.Fatalf("content-type = %q", r.ContentType)
	}
	if r.Body["count"].(float64) != 5 || r.Body["offset"].(float64) != 2 {
		t.Fatalf("count/offset not in body: %v", r.Body)
	}
	cc, ok := r.Body["country_codes"].([]any)
	if !ok || len(cc) != 2 || cc[0] != "US" || cc[1] != "CA" {
		t.Fatalf("country_codes wrong: %v", r.Body["country_codes"])
	}
	// The app credential must never leak into the JSON body.
	if _, present := r.Body["secret"]; present {
		t.Fatalf("secret must not be in the body: %v", r.Body)
	}
}

func TestInstitutionsGet_DefaultCountryCode(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/institutions/get": {200, `{}`}})
	defer srv.Close()
	_, errOut, code, _ := run(t, srv, sandboxEnv(), "institutions", "get")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	cc := reqs[0].Body["country_codes"].([]any)
	if len(cc) != 1 || cc[0] != "US" {
		t.Fatalf("default country_codes should be [US], got %v", cc)
	}
}

func TestInstitutionsGetByID_RequiresInstitutionID(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{})
	defer srv.Close()
	_, _, code, _ := run(t, srv, sandboxEnv(), "institutions", "get-by-id")
	if code != 2 {
		t.Fatalf("missing --institution-id should be a usage error (exit 2), got %d", code)
	}
	if len(reqs) != 0 {
		t.Fatalf("no request should be made when a required flag is missing")
	}
}

// --- access_token as a per-invocation flag ---------------------------------

func TestAccountsGet_PutsAccessTokenInBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/accounts/get": {200, `{"accounts":[]}`}})
	defer srv.Close()
	_, errOut, code, _ := run(t, srv, sandboxEnv(), "accounts", "get", "--access-token", "access-sandbox-1")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	if reqs[0].Body["access_token"] != "access-sandbox-1" {
		t.Fatalf("access_token not in body: %v", reqs[0].Body)
	}
}

func TestItemScopedCommands_RequireAccessToken(t *testing.T) {
	cmds := [][]string{
		{"accounts", "get"},
		{"accounts", "balance"},
		{"auth", "get"},
		{"transactions", "sync"},
		{"identity", "get"},
		{"item", "get"},
		{"item", "remove"},
	}
	for _, args := range cmds {
		var reqs []capturedRequest
		srv := newFake(t, &reqs, map[string]route{})
		_, _, code, _ := run(t, srv, sandboxEnv(), args...)
		srv.Close()
		if code != 2 {
			t.Fatalf("%v without --access-token should exit 2, got %d", args, code)
		}
		if len(reqs) != 0 {
			t.Fatalf("%v should not call the API without --access-token", args)
		}
	}
}

func TestTransactionsSync_CursorAndCount(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/transactions/sync": {200, `{"added":[],"has_more":false}`}})
	defer srv.Close()
	_, errOut, code, _ := run(t, srv, sandboxEnv(),
		"transactions", "sync", "--access-token", "tok", "--cursor", "abc", "--count", "50")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	b := reqs[0].Body
	if b["access_token"] != "tok" || b["cursor"] != "abc" || b["count"].(float64) != 50 {
		t.Fatalf("sync body wrong: %v", b)
	}
}

func TestTransactionsSync_OmitsEmptyCursor(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/transactions/sync": {200, `{}`}})
	defer srv.Close()
	_, _, code, _ := run(t, srv, sandboxEnv(), "transactions", "sync", "--access-token", "tok")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if _, present := reqs[0].Body["cursor"]; present {
		t.Fatalf("empty cursor should be omitted, got %v", reqs[0].Body)
	}
}

func TestTransactionsGet_RequiresDates(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{})
	defer srv.Close()
	_, _, code, _ := run(t, srv, sandboxEnv(), "transactions", "get", "--access-token", "tok", "--start-date", "2026-01-01")
	if code != 2 {
		t.Fatalf("missing --end-date should exit 2, got %d", code)
	}
}

func TestTransactionsGet_BuildsOptions(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/transactions/get": {200, `{"transactions":[]}`}})
	defer srv.Close()
	_, errOut, code, _ := run(t, srv, sandboxEnv(),
		"transactions", "get", "--access-token", "tok",
		"--start-date", "2026-01-01", "--end-date", "2026-02-01", "--count", "25", "--offset", "5")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	b := reqs[0].Body
	if b["start_date"] != "2026-01-01" || b["end_date"] != "2026-02-01" {
		t.Fatalf("dates wrong: %v", b)
	}
	opts := b["options"].(map[string]any)
	if opts["count"].(float64) != 25 || opts["offset"].(float64) != 5 {
		t.Fatalf("options wrong: %v", opts)
	}
}

// --- item exchange + sandbox loop ------------------------------------------

func TestExchangePublicToken(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{
		"/item/public_token/exchange": {200, `{"access_token":"access-sandbox-x","item_id":"item-1"}`},
	})
	defer srv.Close()
	out, errOut, code, _ := run(t, srv, sandboxEnv(), "item", "exchange-public-token", "--public-token", "public-sandbox-1")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	if reqs[0].Body["public_token"] != "public-sandbox-1" {
		t.Fatalf("public_token not sent: %v", reqs[0].Body)
	}
	if !strings.Contains(out, "access-sandbox-x") {
		t.Fatalf("exchange output missing access_token: %q", out)
	}
}

func TestSandboxPublicTokenCreate_Sandbox(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{
		"/sandbox/public_token/create": {200, `{"public_token":"public-sandbox-1"}`},
	})
	defer srv.Close()
	_, errOut, code, _ := run(t, srv, sandboxEnv(),
		"sandbox", "public-token-create", "--institution-id", "ins_109508", "--products", "transactions,auth")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	b := reqs[0].Body
	if b["institution_id"] != "ins_109508" {
		t.Fatalf("institution_id wrong: %v", b)
	}
	prods := b["initial_products"].([]any)
	if len(prods) != 2 || prods[0] != "transactions" || prods[1] != "auth" {
		t.Fatalf("initial_products wrong: %v", prods)
	}
}

func TestSandboxPublicTokenCreate_RefusedInProduction(t *testing.T) {
	// BaseURL still points at the fake server, but PLAID_ENV=production must
	// make the sandbox-only command refuse before any HTTP call.
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/sandbox/public_token/create": {200, `{}`}})
	defer srv.Close()
	env := map[string]string{EnvClientID: "cid", EnvSecret: "sec", EnvEnvironment: "production"}
	_, errOut, code, _ := run(t, srv, env, "sandbox", "public-token-create", "--institution-id", "ins_1")
	if code != 2 {
		t.Fatalf("sandbox create under production should exit 2, got %d", code)
	}
	if len(reqs) != 0 {
		t.Fatalf("no HTTP call should happen when refused")
	}
	if !strings.Contains(errOut, "production") {
		t.Fatalf("refusal message should mention production: %q", errOut)
	}
}

// --- environment resolution ------------------------------------------------

func TestBadEnv_ExitsTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{})
	defer srv.Close()
	env := map[string]string{EnvClientID: "cid", EnvSecret: "sec", EnvEnvironment: "development"}
	_, errOut, code, _ := run(t, srv, env, "institutions", "get")
	if code != 2 {
		t.Fatalf("bad PLAID_ENV should exit 2, got %d", code)
	}
	if !strings.Contains(errOut, "PLAID_ENV") {
		t.Fatalf("error should name PLAID_ENV: %q", errOut)
	}
}

func TestResolveEnv(t *testing.T) {
	cases := map[string]string{"": envSandbox, "sandbox": envSandbox, "SANDBOX": envSandbox, "production": envProduction}
	for in, want := range cases {
		got, err := resolveEnv(in)
		if err != nil || got != want {
			t.Fatalf("resolveEnv(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := resolveEnv("dev"); err == nil {
		t.Fatalf("resolveEnv(dev) should error")
	}
}

func TestProductionBaseURLDerivation(t *testing.T) {
	s := &Service{}
	if got := s.resolveBaseURL(envProduction); got != productionBaseURL {
		t.Fatalf("production base = %q", got)
	}
	if got := s.resolveBaseURL(envSandbox); got != sandboxBaseURL {
		t.Fatalf("sandbox base = %q", got)
	}
}

// --- missing credentials ---------------------------------------------------

func TestMissingCredentials_ExitsOne(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{})
	defer srv.Close()
	_, errOut, code, _ := run(t, srv, map[string]string{EnvClientID: "", EnvSecret: ""}, "institutions", "get")
	if code != 1 {
		t.Fatalf("missing credentials should exit 1, got %d", code)
	}
	if !strings.Contains(errOut, "PLAID_CLIENT_ID") {
		t.Fatalf("error should name the missing credential: %q", errOut)
	}
}

// --- error envelope + credential classification ----------------------------

func TestAPIError_PlainAndJSON(t *testing.T) {
	body := `{"error_type":"INVALID_INPUT","error_code":"INVALID_ACCESS_TOKEN","error_message":"provided access token is invalid","request_id":"req-9"}`
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/accounts/get": {400, body}})
	defer srv.Close()

	// Plain mode: message carries error_code + message; exit 1.
	_, errOut, code, rejected := run(t, srv, sandboxEnv(), "accounts", "get", "--access-token", "bad")
	if code != 1 {
		t.Fatalf("API error should exit 1, got %d", code)
	}
	// INVALID_ACCESS_TOKEN is an Item-token problem, NOT the stored app credential.
	if rejected {
		t.Fatalf("INVALID_ACCESS_TOKEN must NOT mark the stored credential rejected")
	}
	if !strings.Contains(errOut, "INVALID_ACCESS_TOKEN") {
		t.Fatalf("plain error should carry error_code: %q", errOut)
	}

	// JSON mode: structured envelope carries the Plaid fields.
	_, errJSON, _, _ := run(t, srv, sandboxEnv(), "accounts", "get", "--access-token", "bad", "--json")
	var env struct {
		Error struct {
			Kind      string `json:"kind"`
			Status    int    `json:"status"`
			ErrorType string `json:"error_type"`
			ErrorCode string `json:"error_code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errJSON)), &env); err != nil {
		t.Fatalf("json error not decodable: %v (%q)", err, errJSON)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Fatalf("json error envelope wrong: %+v", env.Error)
	}
	if env.Error.ErrorCode != "INVALID_ACCESS_TOKEN" || env.Error.ErrorType != "INVALID_INPUT" || env.Error.RequestID != "req-9" {
		t.Fatalf("json error missing Plaid fields: %+v", env.Error)
	}
}

func TestInvalidAPIKeys_MarksCredentialRejected(t *testing.T) {
	body := `{"error_type":"INVALID_INPUT","error_code":"INVALID_API_KEYS","error_message":"invalid client_id or secret provided","request_id":"req-1"}`
	var reqs []capturedRequest
	srv := newFake(t, &reqs, map[string]route{"/institutions/get": {400, body}})
	defer srv.Close()
	_, _, code, rejected := run(t, srv, sandboxEnv(), "institutions", "get")
	if code != 1 {
		t.Fatalf("exit should be 1, got %d", code)
	}
	if !rejected {
		t.Fatalf("INVALID_API_KEYS must mark the stored credential rejected")
	}
}
