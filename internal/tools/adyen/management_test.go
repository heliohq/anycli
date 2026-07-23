package adyen

import (
	"net/http"
	"strings"
	"testing"
)

func TestMerchantList_Pagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[],"itemsTotal":0,"pagesTotal":0}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "merchant", "list", "--page-size", "50", "--page", "2")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/merchants" {
		t.Errorf("request = %s %s, want GET /merchants", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("pageSize") != "50" || q.Get("pageNumber") != "2" {
		t.Errorf("query = %q, want pageSize=50&pageNumber=2", got.Query)
	}
}

func TestMerchantList_NoPaginationFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "merchant", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	// Unset pagination flags must not be sent — let Adyen apply its defaults.
	q := parseQuery(t, got.Query)
	if _, ok := q["pageSize"]; ok {
		t.Errorf("pageSize should be absent when unset, query = %q", got.Query)
	}
	if _, ok := q["pageNumber"]; ok {
		t.Errorf("pageNumber should be absent when unset, query = %q", got.Query)
	}
}

func TestMerchantGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"MERCHANT_A","name":"Acme"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "management", "merchant", "get", "MERCHANT_A")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/merchants/MERCHANT_A" {
		t.Errorf("path = %q, want /merchants/MERCHANT_A", got.Path)
	}
	if !strings.Contains(stdout, `"id":"MERCHANT_A"`) {
		t.Errorf("stdout = %q, want passthrough", stdout)
	}
}

func TestCompanyList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "company", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/companies" {
		t.Errorf("path = %q, want /companies", got.Path)
	}
}

func TestCompanyGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"COMPANY_A"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "company", "get", "COMPANY_A")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/companies/COMPANY_A" {
		t.Errorf("path = %q, want /companies/COMPANY_A", got.Path)
	}
}

func TestPaymentMethodsList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "payment-methods", "list", "MERCHANT_A")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/merchants/MERCHANT_A/paymentMethodSettings" {
		t.Errorf("path = %q, want /merchants/MERCHANT_A/paymentMethodSettings", got.Path)
	}
}

func TestWebhookList_Merchant(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "webhook", "list", "--merchant", "MERCHANT_A")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/merchants/MERCHANT_A/webhooks" {
		t.Errorf("path = %q, want /merchants/MERCHANT_A/webhooks", got.Path)
	}
}

func TestWebhookList_Company(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "webhook", "list", "--company", "COMPANY_A")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/companies/COMPANY_A/webhooks" {
		t.Errorf("path = %q, want /companies/COMPANY_A/webhooks", got.Path)
	}
}

// Exactly one of --merchant / --company is required for webhook commands.
func TestWebhookList_RequiresScope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "management", "webhook", "list")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage error)", code)
	}
	if !strings.Contains(stderr, "--merchant") || !strings.Contains(stderr, "--company") {
		t.Errorf("stderr = %q, want the scope-required usage message", stderr)
	}
}

func TestWebhookList_RejectsBothScopes(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "management", "webhook", "list", "--merchant", "M", "--company", "C")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage error)", code)
	}
	if !strings.Contains(stderr, "--merchant") || !strings.Contains(stderr, "--company") {
		t.Errorf("stderr = %q, want the mutually-exclusive message", stderr)
	}
}

func TestWebhookGet_Merchant(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"WH1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "webhook", "get", "--merchant", "MERCHANT_A", "WH1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/merchants/MERCHANT_A/webhooks/WH1" {
		t.Errorf("path = %q, want /merchants/MERCHANT_A/webhooks/WH1", got.Path)
	}
}

func TestStoreList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "store", "list", "MERCHANT_A")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/merchants/MERCHANT_A/stores" {
		t.Errorf("path = %q, want /merchants/MERCHANT_A/stores", got.Path)
	}
}

// Terminals live at the TOP-LEVEL /terminals endpoint (Management v3), filtered
// by merchantIds — not nested under /merchants/{id}/terminals.
func TestTerminalList_TopLevel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "terminal", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/terminals" {
		t.Errorf("path = %q, want /terminals", got.Path)
	}
}

func TestTerminalList_MerchantFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "management", "terminal", "list", "--merchant", "MERCHANT_A", "--page-size", "10")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/terminals" {
		t.Errorf("path = %q, want /terminals", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("merchantIds") != "MERCHANT_A" {
		t.Errorf("merchantIds = %q, want MERCHANT_A", q.Get("merchantIds"))
	}
	if q.Get("pageSize") != "10" {
		t.Errorf("pageSize = %q, want 10", q.Get("pageSize"))
	}
}
