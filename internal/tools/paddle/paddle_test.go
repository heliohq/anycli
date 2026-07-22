package paddle

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBaseURLForKey(t *testing.T) {
	cases := []struct {
		name  string
		token string
		env   string
		want  string
	}{
		{"live prefix", "pdl_live_apikey_abc", "", liveBaseURL},
		{"sandbox prefix", "pdl_sdbx_apikey_abc", "", sandboxBaseURL},
		{"legacy defaults live", "0123456789abcdef", "", liveBaseURL},
		{"legacy env override sandbox", "0123456789abcdef", "sandbox", sandboxBaseURL},
		{"prefix beats env", "pdl_live_apikey_abc", "sandbox", liveBaseURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := baseURLForKey(tc.token, tc.env); got != tc.want {
				t.Fatalf("baseURLForKey(%q,%q) = %q, want %q", tc.token, tc.env, got, tc.want)
			}
		})
	}
}

func TestListSendsAuthVersionAndQuery(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /subscriptions": {status: 200, body: `{"data":[{"id":"sub_1"}],"meta":{"request_id":"req_1","pagination":{"next":"https://x/next?after=sub_1","has_more":true}}}`},
	})
	defer srv.Close()

	out, errOut, res := runPaddle(t, srv, nil,
		"subscription", "list", "--status", "active", "--customer-id", "ctm_9", "--per-page", "2", "--filter", "price_id=pri_7")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/subscriptions")
	if req == nil {
		t.Fatal("no GET /subscriptions request recorded")
	}
	if req.Auth != "Bearer pdl_live_apikey_test" {
		t.Errorf("Authorization = %q, want Bearer pdl_live_apikey_test", req.Auth)
	}
	if req.Version != "1" {
		t.Errorf("Paddle-Version = %q, want 1", req.Version)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.Accept)
	}
	if got := req.Query["status"]; len(got) == 0 || got[0] != "active" {
		t.Errorf("status query = %v, want active", got)
	}
	if got := req.Query["customer_id"]; len(got) == 0 || got[0] != "ctm_9" {
		t.Errorf("customer_id query = %v, want ctm_9", got)
	}
	if got := req.Query["per_page"]; len(got) == 0 || got[0] != "2" {
		t.Errorf("per_page query = %v, want 2", got)
	}
	if got := req.Query["price_id"]; len(got) == 0 || got[0] != "pri_7" {
		t.Errorf("price_id filter query = %v, want pri_7", got)
	}
	// Default (non --json) prints the data array.
	if !strings.Contains(out, "sub_1") {
		t.Errorf("stdout = %q, want it to contain sub_1", out)
	}
}

func TestJSONModeEmitsDataAndMeta(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /products": {status: 200, body: `{"data":[{"id":"pro_1"}],"meta":{"request_id":"req_2","pagination":{"next":"cur","has_more":false}}}`},
	})
	defer srv.Close()

	out, _, res := runPaddle(t, srv, nil, "product", "list", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	got := decodeJSON(t, out)
	if _, ok := got["data"]; !ok {
		t.Errorf("json output missing data: %q", out)
	}
	if _, ok := got["meta"]; !ok {
		t.Errorf("json output missing meta: %q", out)
	}
}

func TestGetBuildsPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers/ctm_5": {status: 200, body: `{"data":{"id":"ctm_5"}}`},
	})
	defer srv.Close()

	_, errOut, res := runPaddle(t, srv, nil, "customer", "get", "ctm_5")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	if findReq(reqs, "GET", "/customers/ctm_5") == nil {
		t.Fatal("no GET /customers/ctm_5 recorded")
	}
}

func TestCreateSendsJSONBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /customers": {status: 201, body: `{"data":{"id":"ctm_new"}}`},
	})
	defer srv.Close()

	_, errOut, res := runPaddle(t, srv, nil, "customer", "create", "--data", `{"email":"a@b.com","name":"A"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/customers")
	if req == nil {
		t.Fatal("no POST /customers recorded")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["email"] != "a@b.com" {
		t.Errorf("body email = %v, want a@b.com", body["email"])
	}
}

func TestUpdatePatchesPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /subscriptions/sub_2": {status: 200, body: `{"data":{"id":"sub_2"}}`},
	})
	defer srv.Close()

	_, errOut, res := runPaddle(t, srv, nil, "subscription", "update", "sub_2", "--data", `{"proration_billing_mode":"prorated_immediately"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	if findReq(reqs, "PATCH", "/subscriptions/sub_2") == nil {
		t.Fatal("no PATCH /subscriptions/sub_2 recorded")
	}
}

func TestSubscriptionLifecycleAndPreviewPaths(t *testing.T) {
	cases := []struct {
		args   []string
		method string
		path   string
	}{
		{[]string{"subscription", "cancel", "sub_1"}, "POST", "/subscriptions/sub_1/cancel"},
		{[]string{"subscription", "pause", "sub_1"}, "POST", "/subscriptions/sub_1/pause"},
		{[]string{"subscription", "resume", "sub_1"}, "POST", "/subscriptions/sub_1/resume"},
		{[]string{"subscription", "activate", "sub_1"}, "POST", "/subscriptions/sub_1/activate"},
		{[]string{"subscription", "charge", "sub_1", "--data", `{"items":[]}`}, "POST", "/subscriptions/sub_1/charge"},
		{[]string{"subscription", "preview-charge", "sub_1", "--data", `{"items":[]}`}, "POST", "/subscriptions/sub_1/charge/preview"},
		{[]string{"subscription", "preview-update", "sub_1", "--data", `{"items":[]}`}, "PATCH", "/subscriptions/sub_1/preview"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				tc.method + " " + tc.path: {status: 200, body: `{"data":{"id":"sub_1"}}`},
			})
			defer srv.Close()
			_, errOut, res := runPaddle(t, srv, nil, tc.args...)
			if res.ExitCode != 0 {
				t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
			}
			if findReq(reqs, tc.method, tc.path) == nil {
				t.Fatalf("no %s %s recorded", tc.method, tc.path)
			}
		})
	}
}

func TestTransactionInvoiceAndPreviewPaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /transactions/txn_1/invoice": {status: 200, body: `{"data":{"url":"https://paddle/inv.pdf"}}`},
		"POST /transactions/preview":      {status: 200, body: `{"data":{"details":{}}}`},
	})
	defer srv.Close()

	_, errOut, res := runPaddle(t, srv, nil, "transaction", "invoice", "txn_1")
	if res.ExitCode != 0 {
		t.Fatalf("invoice exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	if findReq(reqs, "GET", "/transactions/txn_1/invoice") == nil {
		t.Fatal("no GET /transactions/txn_1/invoice recorded")
	}
	_, errOut, res = runPaddle(t, srv, nil, "transaction", "preview", "--data", `{"items":[]}`)
	if res.ExitCode != 0 {
		t.Fatalf("preview exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	if findReq(reqs, "POST", "/transactions/preview") == nil {
		t.Fatal("no POST /transactions/preview recorded")
	}
}

func TestEventTypesPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /event-types": {status: 200, body: `{"data":[{"name":"transaction.completed"}]}`},
	})
	defer srv.Close()
	_, errOut, res := runPaddle(t, srv, nil, "event", "types")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	if findReq(reqs, "GET", "/event-types") == nil {
		t.Fatal("no GET /event-types recorded")
	}
}

func TestAPIErrorEnvelopeExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers/ctm_x": {status: 400, headers: map[string]string{"Retry-After": "5"}, body: `{"error":{"type":"request_error","code":"bad_request","detail":"invalid id","documentation_url":"https://developer.paddle.com/errors","errors":[{"field":"id","message":"bad"}]},"meta":{"request_id":"req_9"}}`},
	})
	defer srv.Close()

	out, errOut, res := runPaddle(t, srv, nil, "customer", "get", "ctm_x", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Errorf("400 must not reject the credential")
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout should be empty on error, got %q", out)
	}
	env := decodeJSON(t, errOut)
	e := env["error"].(map[string]any)
	if e["code"] != "bad_request" {
		t.Errorf("error.code = %v, want bad_request", e["code"])
	}
	if e["documentation_url"] != "https://developer.paddle.com/errors" {
		t.Errorf("error.documentation_url = %v", e["documentation_url"])
	}
	if e["request_id"] != "req_9" {
		t.Errorf("error.request_id = %v, want req_9", e["request_id"])
	}
	if e["retry_after"] != "5" {
		t.Errorf("error.retry_after = %v, want 5", e["retry_after"])
	}
	if _, ok := e["errors"]; !ok {
		t.Errorf("error.errors[] missing: %q", errOut)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /products": {status: 403, body: `{"error":{"type":"request_error","code":"forbidden","detail":"nope"},"meta":{"request_id":"req_401"}}`},
	})
	defer srv.Close()

	_, _, res := runPaddle(t, srv, nil, "product", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Errorf("403 must reject the credential")
	}
}

func TestUsageErrorsExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	cases := []struct {
		name string
		args []string
	}{
		{"missing --data on create", []string{"customer", "create"}},
		{"invalid --data JSON", []string{"customer", "create", "--data", "{not json"}},
		{"unknown subcommand", []string{"customer", "frobnicate"}},
		{"bad filter", []string{"customer", "list", "--filter", "novalue"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, res := runPaddle(t, srv, nil, tc.args...)
			if res.ExitCode != 2 {
				t.Fatalf("exit = %d, want 2", res.ExitCode)
			}
		})
	}
}

func TestMissingTokenExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	_, errOut, res := runPaddle(t, srv, map[string]string{}, "product", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errOut, EnvToken) {
		t.Errorf("stderr = %q, want it to mention %s", errOut, EnvToken)
	}
}

func TestSandboxRoutingViaKeyPrefix(t *testing.T) {
	// A sandbox-prefixed key with no base override must route to the sandbox
	// base; here we only assert baseURLForKey since the fake server is used for
	// the request path. Covered by TestBaseURLForKey; this guards the env seam.
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /products": {status: 200, body: `{"data":[]}`}})
	defer srv.Close()
	// With SetBaseURL the prefix routing is bypassed (test seam), so just prove
	// a sandbox key still authenticates and runs.
	_, errOut, res := runPaddle(t, srv, map[string]string{EnvToken: "pdl_sdbx_apikey_test"}, "product", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/products")
	if req == nil || req.Auth != "Bearer pdl_sdbx_apikey_test" {
		t.Fatalf("sandbox key did not authenticate as expected: %+v", req)
	}
}
