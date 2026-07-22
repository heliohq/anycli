package recurly

import (
	"encoding/json"
	"strings"
	"testing"
)

const testKey = "abc123secret"

// decodeJSON unmarshals s into a generic object, failing the test on error.
func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("stdout is not a JSON object: %v (%q)", err, s)
	}
	return m
}

func TestAccountList_PathAuthVersionAndListEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts": {status: 200, body: `{"object":"list","has_more":true,` +
			`"next":"https://v3.recurly.com/accounts?cursor=CURSOR123&limit=5",` +
			`"data":[{"object":"account","code":"bob"}]}`},
	})
	defer srv.Close()

	out, _, res := runService(t, srv, testKey, "", "account", "list", "--limit", "5")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "GET", "/accounts")
	if req == nil {
		t.Fatal("no GET /accounts request recorded")
	}
	if req.Auth != basicAuth(testKey) {
		t.Errorf("Authorization = %q, want %q", req.Auth, basicAuth(testKey))
	}
	if req.Accept != apiVersionAccept {
		t.Errorf("Accept = %q, want %q", req.Accept, apiVersionAccept)
	}
	if got := req.Query.Get("limit"); got != "5" {
		t.Errorf("limit query = %q, want 5", got)
	}
	env := decodeJSON(t, out)
	data, ok := env["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("data = %v, want 1 element", env["data"])
	}
	if env["has_more"] != true {
		t.Errorf("has_more = %v, want true", env["has_more"])
	}
	// next must be reduced to the cursor value (feedable to --cursor), not the URL.
	if env["next"] != "CURSOR123" {
		t.Errorf("next = %v, want cursor CURSOR123", env["next"])
	}
}

func TestAccountGet_AliasPrefixPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts/code-bob": {status: 200, body: `{"object":"account","code":"bob"}`},
	})
	defer srv.Close()

	out, _, res := runService(t, srv, testKey, "", "account", "get", "code-bob")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if findReq(reqs, "GET", "/accounts/code-bob") == nil {
		t.Fatal("alias prefix code-bob was not passed through to the path")
	}
	obj := decodeJSON(t, out)
	if obj["code"] != "bob" {
		t.Errorf("get did not pass the resource through verbatim: %q", out)
	}
}

func TestAccountBalanceAndBillingInfo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts/code-bob/balance":      {status: 200, body: `{"object":"account_balance"}`},
		"GET /accounts/code-bob/billing_info": {status: 200, body: `{"object":"billing_info"}`},
	})
	defer srv.Close()

	if _, _, res := runService(t, srv, testKey, "", "account", "balance", "code-bob"); res.ExitCode != 0 {
		t.Fatalf("balance exit = %d, want 0", res.ExitCode)
	}
	if _, _, res := runService(t, srv, testKey, "", "account", "billing-info", "code-bob"); res.ExitCode != 0 {
		t.Fatalf("billing-info exit = %d, want 0", res.ExitCode)
	}
	if findReq(reqs, "GET", "/accounts/code-bob/balance") == nil {
		t.Error("balance path wrong")
	}
	if findReq(reqs, "GET", "/accounts/code-bob/billing_info") == nil {
		t.Error("billing_info path wrong")
	}
}

func TestSubscriptionAccountScopedList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts/code-bob/subscriptions": {status: 200, body: `{"object":"list","has_more":false,"data":[]}`},
	})
	defer srv.Close()

	if _, _, res := runService(t, srv, testKey, "", "subscription", "list", "--account", "code-bob"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if findReq(reqs, "GET", "/accounts/code-bob/subscriptions") == nil {
		t.Fatal("--account did not scope the list path to the account")
	}
}

func TestSubscriptionLifecycleVerbs(t *testing.T) {
	cases := []struct {
		args   []string
		method string
		path   string
	}{
		{[]string{"subscription", "cancel", "uuid-abc"}, "PUT", "/subscriptions/uuid-abc/cancel"},
		{[]string{"subscription", "pause", "uuid-abc", "--cycles", "2"}, "PUT", "/subscriptions/uuid-abc/pause"},
		{[]string{"subscription", "resume", "uuid-abc"}, "PUT", "/subscriptions/uuid-abc/resume"},
		{[]string{"subscription", "terminate", "uuid-abc"}, "DELETE", "/subscriptions/uuid-abc"},
	}
	for _, c := range cases {
		var reqs []capturedRequest
		srv := newMux(t, &reqs, map[string]stub{
			c.method + " " + c.path: {status: 200, body: `{"object":"subscription"}`},
		})
		if _, stderr, res := runService(t, srv, testKey, "", c.args...); res.ExitCode != 0 {
			t.Fatalf("%v exit = %d, want 0 (stderr=%s)", c.args, res.ExitCode, stderr)
		}
		req := findReq(reqs, c.method, c.path)
		if req == nil {
			t.Fatalf("%v did not hit %s %s", c.args, c.method, c.path)
		}
		srv.Close()
	}
}

func TestSubscriptionPauseBodyCarriesCycles(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /subscriptions/uuid-abc/pause": {status: 200, body: `{"object":"subscription"}`},
	})
	defer srv.Close()

	if _, _, res := runService(t, srv, testKey, "", "subscription", "pause", "uuid-abc", "--cycles", "3"); res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "PUT", "/subscriptions/uuid-abc/pause")
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("pause body is not JSON: %v", err)
	}
	if body["remaining_pause_cycles"] != float64(3) {
		t.Errorf("remaining_pause_cycles = %v, want 3", body["remaining_pause_cycles"])
	}
}

func TestSubscriptionCreateSendsBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /subscriptions": {status: 201, body: `{"object":"subscription"}`},
	})
	defer srv.Close()

	payload := `{"plan_code":"gold","currency":"USD","account":{"code":"bob"}}`
	if _, stderr, res := runService(t, srv, testKey, "", "subscription", "create", "--body", payload); res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", res.ExitCode, stderr)
	}
	req := findReq(reqs, "POST", "/subscriptions")
	if req == nil {
		t.Fatal("no POST /subscriptions")
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("create body not JSON: %v", err)
	}
	if body["plan_code"] != "gold" {
		t.Errorf("create body plan_code = %v, want gold", body["plan_code"])
	}
}

func TestCreateRejectsInvalidBodyAsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, res := runService(t, srv, testKey, "", "subscription", "create", "--body", "{not json")
	if res.ExitCode != 2 {
		t.Fatalf("invalid --body exit = %d, want 2 (usage)", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("invalid body must fail before any HTTP call, got %d requests", len(reqs))
	}
}

func TestInvoiceCollectAndLineItems(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /invoices/number-1000/collect":    {status: 200, body: `{"object":"invoice"}`},
		"GET /invoices/number-1000/line_items": {status: 200, body: `{"object":"list","has_more":false,"data":[]}`},
	})
	defer srv.Close()

	if _, _, res := runService(t, srv, testKey, "", "invoice", "collect", "number-1000"); res.ExitCode != 0 {
		t.Fatalf("collect exit = %d, want 0", res.ExitCode)
	}
	if _, _, res := runService(t, srv, testKey, "", "invoice", "line-items", "number-1000"); res.ExitCode != 0 {
		t.Fatalf("line-items exit = %d, want 0", res.ExitCode)
	}
	if findReq(reqs, "PUT", "/invoices/number-1000/collect") == nil {
		t.Error("collect path/method wrong")
	}
	if findReq(reqs, "GET", "/invoices/number-1000/line_items") == nil {
		t.Error("line-items path wrong")
	}
}

func TestPlanCouponSiteAndTransactionList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /plans":        {status: 200, body: `{"object":"list","has_more":false,"data":[]}`},
		"GET /coupons":      {status: 200, body: `{"object":"list","has_more":false,"data":[]}`},
		"GET /sites":        {status: 200, body: `{"object":"list","has_more":false,"data":[]}`},
		"GET /transactions": {status: 200, body: `{"object":"list","has_more":false,"data":[]}`},
	})
	defer srv.Close()

	for _, group := range []string{"plan", "coupon", "site", "transaction"} {
		if _, stderr, res := runService(t, srv, testKey, "", group, "list"); res.ExitCode != 0 {
			t.Fatalf("%s list exit = %d, want 0 (stderr=%s)", group, res.ExitCode, stderr)
		}
	}
	for _, path := range []string{"/plans", "/coupons", "/sites", "/transactions"} {
		if findReq(reqs, "GET", path) == nil {
			t.Errorf("no GET %s", path)
		}
	}
}

func TestErrorEnvelope_JSONMode(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts/missing": {status: 404, body: `{"error":{"type":"not_found","message":"Couldn't find Account"}}`},
	})
	defer srv.Close()

	_, stderr, res := runService(t, srv, testKey, "", "account", "get", "missing", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	env := decodeJSON(t, stderr)
	e, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("stderr has no error object: %q", stderr)
	}
	if e["kind"] != "api" {
		t.Errorf("kind = %v, want api", e["kind"])
	}
	if e["status"] != float64(404) {
		t.Errorf("status = %v, want 404", e["status"])
	}
	if !strings.Contains(e["message"].(string), "not_found") {
		t.Errorf("message %q should carry the provider type/message", e["message"])
	}
}

func TestErrorEnvelope_PlainMode(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts/missing": {status: 404, body: `{"error":{"type":"not_found","message":"Couldn't find Account"}}`},
	})
	defer srv.Close()

	_, stderr, res := runService(t, srv, testKey, "", "account", "get", "missing")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if strings.HasPrefix(strings.TrimSpace(stderr), "{") {
		t.Errorf("plain mode should not emit JSON, got %q", stderr)
	}
	if !strings.Contains(stderr, "not_found") {
		t.Errorf("plain error %q should carry the provider type", stderr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts": {status: 401, body: `{"error":{"type":"unauthorized","message":"bad key"}}`},
	})
	defer srv.Close()

	_, _, res := runService(t, srv, testKey, "", "account", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 should mark the credential rejected")
	}
}

func TestInvalidAPIKeyTypeRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts": {status: 403, body: `{"error":{"type":"invalid_api_key","message":"key invalid"}}`},
	})
	defer srv.Close()

	_, _, res := runService(t, srv, testKey, "", "account", "list")
	if !res.CredentialRejected {
		t.Error("invalid_api_key error type should mark the credential rejected")
	}
}

func TestRateLimitEchoesRetryAfter(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /accounts": {status: 429, body: `{"error":{"type":"rate_limited","message":"slow down"}}`,
			headers: map[string]string{"Retry-After": "30"}},
	})
	defer srv.Close()

	_, stderr, res := runService(t, srv, testKey, "", "account", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("429 is retryable, must NOT reject the credential")
	}
	if !strings.Contains(stderr, "30") {
		t.Errorf("rate-limit error %q should echo Retry-After=30", stderr)
	}
}

func TestMissingKeyFailsFast(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, stderr, res := runService(t, srv, "", "", "account", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("missing key must not make an HTTP call, got %d", len(reqs))
	}
	if !strings.Contains(stderr, EnvKey) {
		t.Errorf("error %q should name %s", stderr, EnvKey)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, res := runService(t, srv, testKey, "", "account", "frobnicate")
	if res.ExitCode != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", res.ExitCode)
	}
}

func TestHostForRegion(t *testing.T) {
	cases := map[string]string{
		"":     "https://v3.recurly.com",
		"us":   "https://v3.recurly.com",
		"US":   "https://v3.recurly.com",
		"eu":   "https://v3.eu.recurly.com",
		"EU":   "https://v3.eu.recurly.com",
		" eu ": "https://v3.eu.recurly.com",
	}
	for region, want := range cases {
		if got := hostForRegion(region); got != want {
			t.Errorf("hostForRegion(%q) = %q, want %q", region, got, want)
		}
	}
}

func TestListFiltersFlowToQuery(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /subscriptions": {status: 200, body: `{"object":"list","has_more":false,"data":[]}`},
	})
	defer srv.Close()

	_, _, res := runService(t, srv, testKey, "",
		"subscription", "list",
		"--state", "active", "--limit", "50", "--order", "desc",
		"--sort", "created_at", "--begin-time", "2021-01-01T00:00:00Z", "--cursor", "CUR")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	q := findReq(reqs, "GET", "/subscriptions").Query
	for k, want := range map[string]string{
		"state": "active", "limit": "50", "order": "desc",
		"sort": "created_at", "begin_time": "2021-01-01T00:00:00Z", "cursor": "CUR",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
}

func TestNewCommandTreeTraversable(t *testing.T) {
	s := &Service{}
	root := s.NewCommandTree()
	if root == nil || root.Use != "recurly" {
		t.Fatalf("NewCommandTree root = %v", root)
	}
	// Every advertised resource group must be present for Inspect/lint traversal.
	for _, group := range []string{"account", "subscription", "invoice", "transaction", "plan", "coupon", "line-item", "site"} {
		if _, _, err := root.Find([]string{group}); err != nil {
			t.Errorf("group %q not found in tree: %v", group, err)
		}
	}
}
