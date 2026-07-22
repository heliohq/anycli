package pennylane

import (
	"strings"
	"testing"
)

func TestMissingTokenExitsOneWithMessage(t *testing.T) {
	r := run(t, nil, "", "customer", "list")
	if r.res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", r.res.ExitCode)
	}
	if !strings.Contains(r.stderr, "PENNYLANE_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q, want missing-token message", r.stderr)
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	r := run(t, nil, "", "customer", "list", "--json")
	if r.res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", r.res.ExitCode)
	}
	env := decodeErrorEnvelope(t, r.stderr)
	if env["kind"] != "usage" {
		t.Fatalf("kind = %v, want usage", env["kind"])
	}
	if !strings.Contains(env["message"].(string), "PENNYLANE_ACCESS_TOKEN") {
		t.Fatalf("message = %v", env["message"])
	}
}

func TestCustomerListInjectsBearerAndAcceptAndPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers": {status: 200, body: `{"items":[{"id":1}],"has_more":false}`},
	})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "list")
	if r.res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	req := findReq(reqs, "GET", "/customers")
	if req == nil {
		t.Fatal("no GET /customers recorded")
	}
	if req.Auth != "Bearer "+testToken {
		t.Fatalf("Authorization = %q, want Bearer %s", req.Auth, testToken)
	}
	if req.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", req.Accept)
	}
	if strings.TrimSpace(r.stdout) != `{"items":[{"id":1}],"has_more":false}` {
		t.Fatalf("stdout = %q, want provider body verbatim", r.stdout)
	}
}

func TestCustomerListPassesQueryParamsOnlyWhenSet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers": {status: 200, body: `{}`},
	})
	defer srv.Close()

	// No flags: query must be empty.
	run(t, srv, testToken, "customer", "list")
	req := findReq(reqs, "GET", "/customers")
	if len(req.Query) != 0 {
		t.Fatalf("bare list query = %v, want empty", req.Query)
	}

	reqs = nil
	run(t, srv, testToken, "customer", "list", "--cursor", "abc", "--limit", "50", "--filter", "name:foo", "--sort", "-id")
	req = findReq(reqs, "GET", "/customers")
	if got := req.Query.Get("cursor"); got != "abc" {
		t.Fatalf("cursor = %q, want abc", got)
	}
	if got := req.Query.Get("limit"); got != "50" {
		t.Fatalf("limit = %q, want 50", got)
	}
	if got := req.Query.Get("filter"); got != "name:foo" {
		t.Fatalf("filter = %q, want name:foo", got)
	}
	if got := req.Query.Get("sort"); got != "-id" {
		t.Fatalf("sort = %q, want -id", got)
	}
}

func TestCustomerGetPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers/42": {status: 200, body: `{"id":42}`},
	})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "get", "42")
	if r.res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if findReq(reqs, "GET", "/customers/42") == nil {
		t.Fatal("no GET /customers/42 recorded")
	}
}

func TestCustomerCreateHitsCompanyCustomersWithBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /company_customers": {status: 201, body: `{"id":7}`},
	})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "create", "--body", `{"name":"ACME","customer_type":"company"}`)
	if r.res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	req := findReq(reqs, "POST", "/company_customers")
	if req == nil {
		t.Fatal("create did not POST /company_customers (asymmetry: there is no POST /customers)")
	}
	if req.ContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", req.ContentType)
	}
	if !strings.Contains(string(req.Body), `"name":"ACME"`) {
		t.Fatalf("body = %s, want the supplied JSON verbatim", req.Body)
	}
}

func TestCreateMissingBodyIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "create")
	if r.res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", r.res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Fatalf("a request was sent despite missing --body: %+v", reqs)
	}
}

func TestCreateInvalidJSONBodyIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "create", "--body", `{not json`)
	if r.res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", r.res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Fatalf("a request was sent despite invalid JSON: %+v", reqs)
	}
}

func TestSupplierPaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /suppliers":   {status: 200, body: `{}`},
		"GET /suppliers/9": {status: 200, body: `{"id":9}`},
	})
	defer srv.Close()

	if r := run(t, srv, testToken, "supplier", "list"); r.res.ExitCode != 0 {
		t.Fatalf("supplier list exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if r := run(t, srv, testToken, "supplier", "get", "9"); r.res.ExitCode != 0 {
		t.Fatalf("supplier get exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if findReq(reqs, "GET", "/suppliers") == nil || findReq(reqs, "GET", "/suppliers/9") == nil {
		t.Fatal("supplier paths not hit")
	}
}

func TestCustomerInvoicePaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customer_invoices":   {status: 200, body: `{}`},
		"GET /customer_invoices/5": {status: 200, body: `{"id":5}`},
		"POST /customer_invoices":  {status: 201, body: `{"id":6}`},
	})
	defer srv.Close()

	if r := run(t, srv, testToken, "customer-invoice", "list"); r.res.ExitCode != 0 {
		t.Fatalf("list exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if r := run(t, srv, testToken, "customer-invoice", "get", "5"); r.res.ExitCode != 0 {
		t.Fatalf("get exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if r := run(t, srv, testToken, "customer-invoice", "create", "--body", `{"customer_id":1}`); r.res.ExitCode != 0 {
		t.Fatalf("create exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if findReq(reqs, "POST", "/customer_invoices") == nil {
		t.Fatal("customer-invoice create did not POST /customer_invoices")
	}
}

func TestSupplierInvoiceIsReadOnly(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /supplier_invoices":   {status: 200, body: `{}`},
		"GET /supplier_invoices/3": {status: 200, body: `{"id":3}`},
	})
	defer srv.Close()

	if r := run(t, srv, testToken, "supplier-invoice", "list"); r.res.ExitCode != 0 {
		t.Fatalf("list exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if r := run(t, srv, testToken, "supplier-invoice", "get", "3"); r.res.ExitCode != 0 {
		t.Fatalf("get exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	// There is no create subcommand for supplier-invoice.
	r := run(t, srv, testToken, "supplier-invoice", "create", "--body", `{}`)
	if r.res.ExitCode != 2 {
		t.Fatalf("supplier-invoice create should be an unknown-command usage error, exit = %d", r.res.ExitCode)
	}
}

func TestProductPaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /products":   {status: 200, body: `{}`},
		"GET /products/8": {status: 200, body: `{"id":8}`},
	})
	defer srv.Close()

	if r := run(t, srv, testToken, "product", "list"); r.res.ExitCode != 0 {
		t.Fatalf("product list exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if r := run(t, srv, testToken, "product", "get", "8"); r.res.ExitCode != 0 {
		t.Fatalf("product get exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
}

func TestTransactionListGetAndCategorize(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /transactions":               {status: 200, body: `{}`},
		"GET /transactions/11":            {status: 200, body: `{"id":11}`},
		"PUT /transactions/11/categories": {status: 200, body: `{"id":11}`},
	})
	defer srv.Close()

	if r := run(t, srv, testToken, "transaction", "list"); r.res.ExitCode != 0 {
		t.Fatalf("transaction list exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	if r := run(t, srv, testToken, "transaction", "get", "11"); r.res.ExitCode != 0 {
		t.Fatalf("transaction get exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	// Categorize body is a JSON ARRAY (not object) — must be accepted.
	r := run(t, srv, testToken, "transaction", "categorize", "11", "--body", `[{"id":59,"weight":"1"}]`)
	if r.res.ExitCode != 0 {
		t.Fatalf("categorize exit = %d (stderr=%s)", r.res.ExitCode, r.stderr)
	}
	req := findReq(reqs, "PUT", "/transactions/11/categories")
	if req == nil {
		t.Fatal("categorize did not PUT /transactions/11/categories")
	}
	if !strings.HasPrefix(strings.TrimSpace(string(req.Body)), "[") {
		t.Fatalf("categorize body = %s, want the JSON array verbatim", req.Body)
	}
}

func TestLedgerReadPaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /trial_balance":   {status: 200, body: `{}`},
		"GET /ledger_entries":  {status: 200, body: `{}`},
		"GET /journals":        {status: 200, body: `{}`},
		"GET /ledger_accounts": {status: 200, body: `{}`},
	})
	defer srv.Close()

	cases := []struct {
		sub  string
		path string
	}{
		{"trial-balance", "/trial_balance"},
		{"entries", "/ledger_entries"},
		{"journals", "/journals"},
		{"accounts", "/ledger_accounts"},
	}
	for _, c := range cases {
		reqs = nil
		r := run(t, srv, testToken, "ledger", c.sub)
		if r.res.ExitCode != 0 {
			t.Fatalf("ledger %s exit = %d (stderr=%s)", c.sub, r.res.ExitCode, r.stderr)
		}
		if findReq(reqs, "GET", c.path) == nil {
			t.Fatalf("ledger %s did not GET %s", c.sub, c.path)
		}
	}
}

func TestAPIErrorExitsOneWithMessage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers/99": {status: 404, body: `{"message":"customer not found"}`},
	})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "get", "99")
	if r.res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (API error)", r.res.ExitCode)
	}
	if !strings.Contains(r.stderr, "customer not found") {
		t.Fatalf("stderr = %q, want provider message", r.stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers/99": {status: 403, body: `{"message":"forbidden"}`},
	})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "get", "99", "--json")
	if r.res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", r.res.ExitCode)
	}
	env := decodeErrorEnvelope(t, r.stderr)
	if env["kind"] != "api" {
		t.Fatalf("kind = %v, want api", env["kind"])
	}
	if status, ok := env["status"].(float64); !ok || int(status) != 403 {
		t.Fatalf("status = %v, want 403", env["status"])
	}
}

func TestUnauthorizedIsCredentialRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /customers": {status: 401, body: `{"message":"invalid token"}`},
	})
	defer srv.Close()

	r := run(t, srv, testToken, "customer", "list")
	if r.res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", r.res.ExitCode)
	}
	if !r.res.CredentialRejected {
		t.Fatal("401 must classify as a credential rejection so the engine invalidates the token")
	}
}

func TestForbiddenIsNotCredentialRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /trial_balance": {status: 403, body: `{"message":"missing scope"}`},
	})
	defer srv.Close()

	r := run(t, srv, testToken, "ledger", "trial-balance")
	if r.res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", r.res.ExitCode)
	}
	if r.res.CredentialRejected {
		t.Fatal("403 (missing scope) must NOT invalidate a token that may still be valid")
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	r := run(t, nil, testToken, "customer", "frobnicate")
	if r.res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", r.res.ExitCode)
	}
}

func TestBareGroupShowsHelpNotSuccessOnUnknown(t *testing.T) {
	// A bare resource group is runnable help (exit 0), but with an extra arg it
	// must fail rather than silently succeed.
	r := run(t, nil, testToken, "customer", "extra-arg")
	if r.res.ExitCode != 2 {
		t.Fatalf("customer <bad-arg> exit = %d, want 2", r.res.ExitCode)
	}
}
