package chargebee

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestBaseURLFromSite(t *testing.T) {
	got := baseURLFromSite("acme-test")
	want := "https://acme-test.chargebee.com/api/v2"
	if got != want {
		t.Fatalf("baseURLFromSite = %q, want %q", got, want)
	}
}

func TestCustomerListSendsBasicAuthAndQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"list":[{"customer":{"id":"c1"}}],"next_offset":"cur2"}`, &got)

	code, stdout, stderr := run(t, srv, "customer", "list",
		"--limit", "1", "--offset", "cur1", "--filter", "status[is]=active")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/customers" {
		t.Fatalf("request = %s %s, want GET /customers", got.Method, got.Path)
	}
	user, pass := decodeBasicUser(t, got.Auth)
	if user != testAPIKey || pass != "" {
		t.Fatalf("basic auth = %q:%q, want api key as username, empty password", user, pass)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "1" || q.Get("offset") != "cur1" || q.Get("status[is]") != "active" {
		t.Fatalf("query = %v, want limit/offset/filter bracket params", q)
	}
	// Native JSON passthrough on stdout.
	if !strings.Contains(stdout, `"next_offset":"cur2"`) {
		t.Fatalf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestCustomerGetHitsResourcePath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"customer":{"id":"c1"}}`, &got)

	if code, _, stderr := run(t, srv, "customer", "get", "c1"); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/customers/c1" {
		t.Fatalf("request = %s %s, want GET /customers/c1", got.Method, got.Path)
	}
}

func TestSubscriptionCreateFormEncodesIndexedItems(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"subscription":{"id":"s1"}}`, &got)

	code, _, stderr := run(t, srv, "subscription", "create",
		"--customer-id", "c1",
		"--item-price", "basic-USD:2",
		"--item-price", "addon-USD",
		"--param", "auto_collection=on")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/customers/c1/subscription_for_items" {
		t.Fatalf("request = %s %s, want POST /customers/c1/subscription_for_items", got.Method, got.Path)
	}
	if !strings.HasPrefix(got.ContentType, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type = %q, want form-urlencoded", got.ContentType)
	}
	// The body must be form-encoded (NOT JSON) and carry Chargebee's bracketed
	// indexed-array encoding for subscription_items.
	if json.Valid(got.Body) && strings.HasPrefix(strings.TrimSpace(string(got.Body)), "{") {
		t.Fatalf("body is JSON, want form-urlencoded: %s", got.Body)
	}
	form := parseQuery(t, string(got.Body))
	if form.Get("subscription_items[item_price_id][0]") != "basic-USD" ||
		form.Get("subscription_items[quantity][0]") != "2" {
		t.Fatalf("indexed item 0 = %v, want basic-USD qty 2", form)
	}
	if form.Get("subscription_items[item_price_id][1]") != "addon-USD" {
		t.Fatalf("indexed item 1 = %v, want addon-USD", form)
	}
	if form.Get("auto_collection") != "on" {
		t.Fatalf("flat param = %v, want auto_collection=on", form)
	}
}

func TestSubscriptionChangeUpdatesForItems(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"subscription":{"id":"s1"}}`, &got)

	if code, _, stderr := run(t, srv, "subscription", "change", "s1", "--item-price", "pro-USD"); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/subscriptions/s1/update_subscription_for_items" {
		t.Fatalf("request = %s %s, want POST /subscriptions/s1/update_subscription_for_items", got.Method, got.Path)
	}
	form := parseQuery(t, string(got.Body))
	if form.Get("subscription_items[item_price_id][0]") != "pro-USD" {
		t.Fatalf("body = %v, want pro-USD item", form)
	}
}

func TestInvoicePDFIsPost(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"download":{"download_url":"https://cb/x.pdf","valid_till":123}}`, &got)

	code, stdout, stderr := run(t, srv, "invoice", "pdf", "inv1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/invoices/inv1/pdf" {
		t.Fatalf("request = %s %s, want POST /invoices/inv1/pdf", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"download_url"`) {
		t.Fatalf("stdout = %q, want download object passthrough", stdout)
	}
}

func TestUsageCreateIsSubscriptionScoped(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"usage":{"id":"u1"}}`, &got)

	code, _, stderr := run(t, srv, "usage", "create",
		"--subscription-id", "s1",
		"--param", "item_price_id=metered-USD",
		"--param", "quantity=5")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/subscriptions/s1/usages" {
		t.Fatalf("request = %s %s, want POST /subscriptions/s1/usages", got.Method, got.Path)
	}
	form := parseQuery(t, string(got.Body))
	if form.Get("item_price_id") != "metered-USD" || form.Get("quantity") != "5" {
		t.Fatalf("body = %v, want metered usage fields", form)
	}
}

func TestUsageListIsTopLevel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"list":[]}`, &got)

	if code, _, stderr := run(t, srv, "usage", "list"); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/usages" {
		t.Fatalf("request = %s %s, want GET /usages", got.Method, got.Path)
	}
}

func TestGetEscapeHatchReadsArbitraryPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"list":[]}`, &got)

	code, _, stderr := run(t, srv, "get", "--path", "/quotes", "--query", "limit=2")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/quotes" {
		t.Fatalf("request = %s %s, want GET /quotes", got.Method, got.Path)
	}
	if parseQuery(t, got.Query).Get("limit") != "2" {
		t.Fatalf("query = %q, want limit=2", got.Query)
	}
}

func TestAPIErrorExitsOneWithEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 400, `{"api_error_code":"resource_not_found","message":"customer not found"}`, &got)

	// Plain text on stderr, exit 1.
	code, _, stderr := run(t, srv, "customer", "get", "missing")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "resource_not_found") || !strings.Contains(stderr, "customer not found") {
		t.Fatalf("stderr = %q, want api_error_code + message", stderr)
	}

	// --json structured envelope, exit 1.
	code, _, stderr = run(t, srv, "customer", "get", "missing", "--json")
	if code != 1 {
		t.Fatalf("json exit = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message      string `json:"message"`
			Kind         string `json:"kind"`
			Status       int    `json:"status"`
			APIErrorCode string `json:"api_error_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 || env.Error.APIErrorCode != "resource_not_found" {
		t.Fatalf("envelope = %+v, want api kind, status 400, api_error_code", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"api_error_code":"api_authentication_failed","message":"bad key"}`, &got)

	result, _, _ := runResult(t, srv, "customer", "list")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 with credential rejection", result)
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)

	cases := [][]string{
		{"customer", "get"},        // missing required id arg
		{"customer", "bogus"},      // unknown subcommand
		{"subscription", "create"}, // missing required --customer-id
		{"usage", "create"},        // missing required --subscription-id
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, _, _ := run(t, srv, args...)
			if code != 2 {
				t.Fatalf("exit = %d, want 2 (usage error)", code)
			}
			if got.Method != "" {
				t.Fatalf("usage error must not reach the API: %s %s", got.Method, got.Path)
			}
		})
	}
}

func TestMissingCredentialsExitOne(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"no api key", map[string]string{EnvSite: testSite}},
		{"no site", map[string]string{EnvAPIKey: testAPIKey}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf strings.Builder
			svc := &Service{Out: &out, Err: &errBuf}
			result, err := svc.Execute(t.Context(), []string{"customer", "list"}, tc.env)
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if result.ExitCode != 1 {
				t.Fatalf("exit = %d, want 1", result.ExitCode)
			}
		})
	}
}

// parseQuery parses a raw query/body string into url.Values.
func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}
