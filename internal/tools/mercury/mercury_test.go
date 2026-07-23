package mercury

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records one request the fake Mercury server received.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Query  map[string][]string
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newMux is a multi-route fake Mercury server keyed by "METHOD /path".
func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reqs = append(*reqs, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Query:  r.URL.Query(),
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
}

// run drives Execute against a fake server and returns exit code, stdout, stderr.
func run(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	svc := &Service{BaseURL: srv.URL + "/api/v1", HC: srv.Client(), Out: &out, Err: &errb}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return res.ExitCode, out.String(), errb.String()
}

// decodeEnvelope parses the {"data":...} stdout envelope.
func decodeEnvelope(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("stdout is not a JSON object: %v (%q)", err, s)
	}
	return m
}

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

func TestAccountList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/accounts": {status: 200, body: `{"accounts":[{"id":"a1","name":"Checking","availableBalance":1200.50}],"page":{"next":null}}`},
	})
	defer srv.Close()

	code, out, _ := run(t, srv, "account", "list", "--limit", "50", "--order", "desc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	req := findReq(reqs, "GET", "/api/v1/accounts")
	if req == nil {
		t.Fatal("no GET /api/v1/accounts request")
	}
	if req.Auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token (no secret-token: prefix)", req.Auth)
	}
	if got := req.Query["limit"]; len(got) != 1 || got[0] != "50" {
		t.Errorf("limit query = %v, want [50]", got)
	}
	if got := req.Query["order"]; len(got) != 1 || got[0] != "desc" {
		t.Errorf("order query = %v, want [desc]", got)
	}
	env := decodeEnvelope(t, out)
	var accts []map[string]any
	if err := json.Unmarshal(env["data"], &accts); err != nil {
		t.Fatalf("data is not an array: %v", err)
	}
	if len(accts) != 1 || accts[0]["id"] != "a1" {
		t.Errorf("data = %v, want one account a1", accts)
	}
	if _, ok := env["page"]; !ok {
		t.Error("expected page meta to pass through")
	}
}

func TestAccountListEmptyBecomesArray(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/accounts": {status: 200, body: `{"page":{"next":null}}`},
	})
	defer srv.Close()
	code, out, _ := run(t, srv, "account", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	env := decodeEnvelope(t, out)
	if string(env["data"]) != "[]" {
		t.Errorf("data = %s, want [] for a missing list", env["data"])
	}
}

func TestAccountGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/account/a1": {status: 200, body: `{"id":"a1","name":"Checking","currentBalance":42}`},
	})
	defer srv.Close()
	code, out, _ := run(t, srv, "account", "get", "a1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if findReq(reqs, "GET", "/api/v1/account/a1") == nil {
		t.Fatal("no GET /api/v1/account/a1")
	}
	env := decodeEnvelope(t, out)
	var obj map[string]any
	if err := json.Unmarshal(env["data"], &obj); err != nil {
		t.Fatalf("data is not an object: %v", err)
	}
	if obj["id"] != "a1" {
		t.Errorf("data.id = %v, want a1", obj["id"])
	}
}

func TestTransactionListRequiresAccount(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	code, _, errb := run(t, srv, "transaction", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage error for missing --account)", code)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made when --account is missing, got %d", len(reqs))
	}
	if !strings.Contains(errb, "account") {
		t.Errorf("stderr = %q, want mention of required account flag", errb)
	}
}

func TestTransactionList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/account/a1/transactions": {status: 200, body: `{"total":2,"transactions":[{"id":"t1","amount":-10},{"id":"t2","amount":5}]}`},
	})
	defer srv.Close()
	code, out, _ := run(t, srv, "transaction", "list", "--account", "a1", "--status", "sent", "--start", "2026-01-01", "--limit", "10", "--offset", "5")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	req := findReq(reqs, "GET", "/api/v1/account/a1/transactions")
	if req == nil {
		t.Fatal("no transactions request")
	}
	for k, want := range map[string]string{"status": "sent", "start": "2026-01-01", "limit": "10", "offset": "5"} {
		if got := req.Query[k]; len(got) != 1 || got[0] != want {
			t.Errorf("query %s = %v, want [%s]", k, got, want)
		}
	}
	env := decodeEnvelope(t, out)
	var txs []map[string]any
	if err := json.Unmarshal(env["data"], &txs); err != nil {
		t.Fatalf("data not array: %v", err)
	}
	if len(txs) != 2 {
		t.Errorf("want 2 transactions, got %d", len(txs))
	}
	if string(env["total"]) != "2" {
		t.Errorf("total meta = %s, want 2", env["total"])
	}
}

func TestTransactionGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/account/a1/transaction/t9": {status: 200, body: `{"id":"t9","amount":-99}`},
	})
	defer srv.Close()
	code, out, _ := run(t, srv, "transaction", "get", "t9", "--account", "a1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if findReq(reqs, "GET", "/api/v1/account/a1/transaction/t9") == nil {
		t.Fatal("no transaction get request at account-scoped path")
	}
	env := decodeEnvelope(t, out)
	var obj map[string]any
	_ = json.Unmarshal(env["data"], &obj)
	if obj["id"] != "t9" {
		t.Errorf("data.id = %v, want t9", obj["id"])
	}
}

func TestRecipientListAndGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/recipients":   {status: 200, body: `{"total":1,"recipients":[{"id":"r1","name":"Vendor"}],"page":{}}`},
		"GET /api/v1/recipient/r1": {status: 200, body: `{"id":"r1","name":"Vendor"}`},
	})
	defer srv.Close()

	code, out, _ := run(t, srv, "recipient", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, want 0", code)
	}
	env := decodeEnvelope(t, out)
	var rs []map[string]any
	_ = json.Unmarshal(env["data"], &rs)
	if len(rs) != 1 || rs[0]["id"] != "r1" {
		t.Errorf("recipients data = %v, want [r1]", rs)
	}

	code, out, _ = run(t, srv, "recipient", "get", "r1")
	if code != 0 {
		t.Fatalf("get exit = %d, want 0", code)
	}
	env = decodeEnvelope(t, out)
	var obj map[string]any
	_ = json.Unmarshal(env["data"], &obj)
	if obj["id"] != "r1" {
		t.Errorf("recipient get data.id = %v, want r1", obj["id"])
	}
}

func TestTreasuryGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/treasury": {status: 200, body: `{"accounts":[{"id":"tr1","currentBalance":1000,"netReturns":12.5}],"page":{}}`},
	})
	defer srv.Close()
	code, out, _ := run(t, srv, "treasury", "get")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	env := decodeEnvelope(t, out)
	var accts []map[string]any
	if err := json.Unmarshal(env["data"], &accts); err != nil {
		t.Fatalf("data not array: %v", err)
	}
	if len(accts) != 1 || accts[0]["id"] != "tr1" {
		t.Errorf("treasury data = %v, want [tr1]", accts)
	}
}

func TestCardListRequiresAccount(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	code, _, _ := run(t, srv, "card", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (missing --account)", code)
	}
	if len(reqs) != 0 {
		t.Errorf("expected no HTTP call, got %d", len(reqs))
	}
}

func TestCardList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/account/a1/cards": {status: 200, body: `{"cards":[{"cardId":"c1","lastFourDigits":"4242","status":"active","type":"virtual"}]}`},
	})
	defer srv.Close()
	code, out, _ := run(t, srv, "card", "list", "--account", "a1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	env := decodeEnvelope(t, out)
	var cards []map[string]any
	if err := json.Unmarshal(env["data"], &cards); err != nil {
		t.Fatalf("data not array: %v", err)
	}
	if len(cards) != 1 || cards[0]["cardId"] != "c1" {
		t.Errorf("cards data = %v, want [c1]", cards)
	}
}

func TestMissingToken(t *testing.T) {
	var out, errb bytes.Buffer
	svc := &Service{Out: &out, Err: &errb}
	res, err := svc.Execute(context.Background(), []string{"account", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 for missing token", res.ExitCode)
	}
	if !strings.Contains(errb.String(), "MERCURY_ACCESS_TOKEN") {
		t.Errorf("stderr = %q, want mention of MERCURY_ACCESS_TOKEN", errb.String())
	}
}

func TestAPIErrorExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/accounts": {status: 500, body: `{"message":"boom"}`},
	})
	defer srv.Close()
	code, _, errb := run(t, srv, "account", "list")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for API error", code)
	}
	if !strings.Contains(errb, "boom") || !strings.Contains(errb, "500") {
		t.Errorf("stderr = %q, want Mercury message + status", errb)
	}
}

func TestCredentialRejectedOn401(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/accounts": {status: 401, body: `{"message":"unauthorized"}`},
	})
	defer srv.Close()
	var out, errb bytes.Buffer
	svc := &Service{BaseURL: srv.URL + "/api/v1", HC: srv.Client(), Out: &out, Err: &errb}
	res, _ := svc.Execute(context.Background(), []string{"account", "list"}, map[string]string{EnvToken: "bad"})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("CredentialRejected = false, want true so the token gateway refreshes")
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /api/v1/accounts": {status: 403, body: `{"message":"forbidden"}`},
	})
	defer srv.Close()
	code, _, errb := run(t, srv, "account", "list", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	line := strings.TrimSpace(errb)
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("stderr under --json is not a JSON error envelope: %v (%q)", err, line)
	}
	if env.Error.Kind != "api" || env.Error.Status != 403 {
		t.Errorf("error envelope = %+v, want kind=api status=403", env.Error)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	code, _, _ := run(t, srv, "account", "bogus")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", code)
	}
}

// TestNewCommandTreeTraversable proves the design-318 seam builds without a
// token (used by Inspect/lint), covering every group.
func TestNewCommandTreeTraversable(t *testing.T) {
	root := (&Service{}).NewCommandTree()
	if root == nil {
		t.Fatal("NewCommandTree returned nil")
	}
	groups := map[string]bool{}
	for _, c := range root.Commands() {
		groups[c.Name()] = true
	}
	for _, want := range []string{"account", "transaction", "recipient", "treasury", "card"} {
		if !groups[want] {
			t.Errorf("missing group %q", want)
		}
	}
}
