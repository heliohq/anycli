package stripe

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- reads: auth, version pin, path/method ---

func TestBalanceGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"balance","available":[]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "balance", "get")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/balance" {
		t.Errorf("request = %s %s, want GET /balance", got.Method, got.Path)
	}
	if got.Auth != "Bearer sk_test_123" {
		t.Errorf("Authorization = %q, want Bearer sk_test_123", got.Auth)
	}
	if got.StripeVersion != stripeVersion {
		t.Errorf("Stripe-Version = %q, want %q", got.StripeVersion, stripeVersion)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if strings.TrimSpace(stdout) != `{"object":"balance","available":[]}` {
		t.Errorf("stdout = %q, want the response verbatim", stdout)
	}
}

func TestBalanceTransactionsPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "balance", "transactions", "--limit", "5", "--starting-after", "txn_9")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/balance_transactions" {
		t.Errorf("path = %q, want /balance_transactions", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "5" || q.Get("starting_after") != "txn_9" {
		t.Errorf("query = %v, want limit=5 starting_after=txn_9", q)
	}
}

func TestChargeListFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "charge", "list", "--limit", "3", "--ending-before", "ch_2", "--param", "customer=cus_1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/charges" {
		t.Errorf("path = %q, want /charges", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "3" || q.Get("ending_before") != "ch_2" || q.Get("customer") != "cus_1" {
		t.Errorf("query = %v, want limit=3 ending_before=ch_2 customer=cus_1", q)
	}
}

func TestChargeGetByID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"ch_1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "charge", "get", "ch_1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/charges/ch_1" {
		t.Errorf("request = %s %s, want GET /charges/ch_1", got.Method, got.Path)
	}
}

func TestPaymentIntentListReadOnly(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"list","data":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "payment-intent", "list")
	if exit != 0 || got.Path != "/payment_intents" {
		t.Errorf("exit=%d path=%q, want 0 /payment_intents", exit, got.Path)
	}
}

// --- mutations: form body, Content-Type, Idempotency-Key ---

func TestCustomerCreateFormAndIdempotency(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"cus_1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "customer", "create",
		"--param", "email=a@b.com", "--param", "name=Acme",
		"--idempotency-key", "key-123")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/customers" {
		t.Errorf("request = %s %s, want POST /customers", got.Method, got.Path)
	}
	if got.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got.ContentType)
	}
	if got.IdempotencyKey != "key-123" {
		t.Errorf("Idempotency-Key = %q, want key-123", got.IdempotencyKey)
	}
	form := parseForm(t, got.Body)
	if form.Get("email") != "a@b.com" || form.Get("name") != "Acme" {
		t.Errorf("form = %v, want email + name", form)
	}
}

func TestCustomerUpdateIsPost(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"cus_1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "customer", "update", "cus_1", "--param", "name=New")
	if exit != 0 || got.Method != http.MethodPost || got.Path != "/customers/cus_1" {
		t.Errorf("exit=%d request=%s %s, want 0 POST /customers/cus_1", exit, got.Method, got.Path)
	}
}

func TestCustomerSearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"search_result","data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "customer", "search", "--query", "email:'a@b.com'")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/customers/search" {
		t.Errorf("request = %s %s, want GET /customers/search", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("query") != "email:'a@b.com'" {
		t.Errorf("query param = %q, want the search string", q.Get("query"))
	}
}

func TestInvoiceFinalizeActionPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"in_1","status":"open"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "invoice", "finalize", "in_1")
	if exit != 0 || got.Method != http.MethodPost || got.Path != "/invoices/in_1/finalize" {
		t.Errorf("exit=%d request=%s %s, want 0 POST /invoices/in_1/finalize", exit, got.Method, got.Path)
	}
	// Even a parameterless mutation sends a form Content-Type.
	if got.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want form", got.ContentType)
	}
}

func TestInvoiceSendActionPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"in_1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "invoice", "send", "in_1")
	if exit != 0 || got.Path != "/invoices/in_1/send" {
		t.Errorf("exit=%d path=%q, want 0 /invoices/in_1/send", exit, got.Path)
	}
}

func TestSubscriptionCancelIsDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"sub_1","status":"canceled"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "subscription", "cancel", "sub_1")
	if exit != 0 || got.Method != http.MethodDelete || got.Path != "/subscriptions/sub_1" {
		t.Errorf("exit=%d request=%s %s, want 0 DELETE /subscriptions/sub_1", exit, got.Method, got.Path)
	}
}

func TestRefundCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"re_1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "refund", "create", "--param", "charge=ch_1", "--param", "amount=500")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/refunds" {
		t.Errorf("request = %s %s, want POST /refunds", got.Method, got.Path)
	}
	form := parseForm(t, got.Body)
	if form.Get("charge") != "ch_1" || form.Get("amount") != "500" {
		t.Errorf("form = %v, want charge + amount", form)
	}
}

// --- top-level search + get passthrough ---

func TestTopLevelSearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"search_result","data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "search", "--resource", "charges", "--query", "amount>1000", "--limit", "2")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/charges/search" {
		t.Errorf("path = %q, want /charges/search", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("query") != "amount>1000" || q.Get("limit") != "2" {
		t.Errorf("query = %v, want query + limit", q)
	}
}

func TestTopLevelSearchUnknownResourceIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "search", "--resource", "widgets", "--query", "x")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want no request made", got.Path)
	}
	if !strings.Contains(stderr, "resource") {
		t.Errorf("stderr = %q, want a resource hint", stderr)
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "customer", "search")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 (missing --query)", exit)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want no request", got.Path)
	}
}

func TestGetPassthroughNormalizesPath(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{"bare", "account", "/account"},
		{"leading slash", "/account", "/account"},
		{"v1 prefix", "/v1/charges", "/charges"},
		{"v1 no slash", "v1/charges", "/charges"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{"ok":true}`, &got)
			defer srv.Close()
			exit, _, stderr := run(t, srv, "get", c.arg)
			if exit != 0 {
				t.Fatalf("exit = %d, stderr = %s", exit, stderr)
			}
			if got.Method != http.MethodGet || got.Path != c.want {
				t.Errorf("request = %s %s, want GET %s", got.Method, got.Path, c.want)
			}
		})
	}
}

// --- errors ---

func TestAPIErrorRendersTypedEnvelope(t *testing.T) {
	body := `{"error":{"type":"invalid_request_error","code":"resource_missing","message":"No such charge","param":"id"}}`
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, body, &got)
	defer srv.Close()

	// plain text
	exit, stdout, stderr := run(t, srv, "charge", "get", "ch_missing")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	if !strings.Contains(stderr, "No such charge") || !strings.Contains(stderr, "resource_missing") {
		t.Errorf("stderr = %q, want Stripe error type/code/message", stderr)
	}

	// --json envelope
	_, _, jsonErr := run(t, srv, "charge", "get", "ch_missing", "--json")
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonErr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, jsonErr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusNotFound {
		t.Errorf("envelope = %+v, want kind=api status=404", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	body := `{"error":{"type":"invalid_request_error","message":"Invalid API Key provided"}}`
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, body, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "balance", "get")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("want CredentialRejected true on 401")
	}
}

func TestForbiddenDoesNotRejectCredential(t *testing.T) {
	body := `{"error":{"type":"invalid_request_error","message":"insufficient permissions"}}`
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, body, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "balance", "get")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("403 must not reject the credential")
	}
}

func TestMissingTokenExitsOneWithJSONEnvelope(t *testing.T) {
	result, stdout, stderr := runNoToken(t, "balance", "get", "--json")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "STRIPE_ACCESS_TOKEN") || !strings.Contains(stderr, `"kind"`) {
		t.Errorf("stderr = %q, want a JSON usage envelope naming the env var", stderr)
	}
}

func TestBadParamSyntaxIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "customer", "create", "--param", "novalue")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 (bad --param)", exit)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want no request", got.Path)
	}
}

func TestLimitOutOfRangeIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "charge", "list", "--limit", "500")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 (limit out of range)", exit)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want no request", got.Path)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "charge", "bogus")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 (unknown subcommand)", exit)
	}
}
