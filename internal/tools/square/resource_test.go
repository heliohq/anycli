package square

import (
	"net/http"
	"testing"
)

// TestPaymentListQuery proves the read filters map to query params.
func TestPaymentListQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"payments":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "payment", "list",
		"--location-id", "L1", "--begin-time", "2026-01-01T00:00:00Z",
		"--sort-order", "DESC", "--limit", "25")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %s)", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v2/payments" {
		t.Fatalf("got %s %s, want GET /v2/payments", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("location_id") != "L1" {
		t.Errorf("location_id = %q, want L1", q.Get("location_id"))
	}
	if q.Get("begin_time") != "2026-01-01T00:00:00Z" {
		t.Errorf("begin_time = %q", q.Get("begin_time"))
	}
	if q.Get("sort_order") != "DESC" {
		t.Errorf("sort_order = %q, want DESC", q.Get("sort_order"))
	}
	if q.Get("limit") != "25" {
		t.Errorf("limit = %q, want 25", q.Get("limit"))
	}
}

// TestOrderSearchPostsBodyVerbatim proves search is a POST carrying the raw
// --body JSON to /v2/orders/search.
func TestOrderSearchPostsBodyVerbatim(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"orders":[]}`, &got)
	defer srv.Close()

	body := `{"location_ids":["L1"],"limit":10}`
	exit, _, stderr := run(t, srv, "order", "search", "--body", body)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %s)", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/orders/search" {
		t.Fatalf("got %s %s, want POST /v2/orders/search", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ContentType)
	}
	decoded := decodeBody(t, got.Body)
	if _, ok := decoded["location_ids"]; !ok {
		t.Errorf("body did not carry location_ids: %s", got.Body)
	}
}

// TestCustomerCreatePostsBody proves create posts to /v2/customers.
func TestCustomerCreatePostsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"customer":{"id":"C1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "customer", "create", "--body", `{"given_name":"Ada"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %s)", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/customers" {
		t.Fatalf("got %s %s, want POST /v2/customers", got.Method, got.Path)
	}
	decoded := decodeBody(t, got.Body)
	if decoded["given_name"] != "Ada" {
		t.Errorf("given_name = %v, want Ada", decoded["given_name"])
	}
}

// TestCustomerUpdateUsesPut proves update maps to PUT /v2/customers/{id}.
func TestCustomerUpdateUsesPut(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"customer":{"id":"C1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "customer", "update", "--customer-id", "C1", "--body", `{"note":"vip"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPut || got.Path != "/v2/customers/C1" {
		t.Fatalf("got %s %s, want PUT /v2/customers/C1", got.Method, got.Path)
	}
}

// TestInvoiceListRequiresLocation proves list sends location_id and rejects its
// absence as a usage error (never reaching the server).
func TestInvoiceListRequiresLocation(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"invoices":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "invoice", "list")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (missing required --location-id)", exit)
	}
	if got.Method != "" {
		t.Errorf("server should not be reached on a missing-required-flag error")
	}

	got = capturedRequest{}
	exit, _, _ = run(t, srv, "invoice", "list", "--location-id", "L9")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	q := parseQuery(t, got.Query)
	if q.Get("location_id") != "L9" {
		t.Errorf("location_id = %q, want L9", q.Get("location_id"))
	}
}

// TestInvoicePublishPath proves publish posts to /v2/invoices/{id}/publish.
func TestInvoicePublishPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"invoice":{"id":"INV1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "invoice", "publish", "--invoice-id", "INV1", "--body", `{"version":0,"idempotency_key":"k1"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/invoices/INV1/publish" {
		t.Fatalf("got %s %s, want POST /v2/invoices/INV1/publish", got.Method, got.Path)
	}
}

// TestInventoryGetBatchRetrieve proves inventory read posts to the batch endpoint.
func TestInventoryGetBatchRetrieve(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"counts":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "inventory", "get", "--body", `{"catalog_object_ids":["V1"]}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/inventory/counts/batch-retrieve" {
		t.Fatalf("got %s %s, want POST /v2/inventory/counts/batch-retrieve", got.Method, got.Path)
	}
}

// TestCatalogGetIncludeRelated proves the include-related flag maps to the query.
func TestCatalogGetIncludeRelated(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":{"id":"O1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "catalog", "get", "--object-id", "O1", "--include-related")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Path != "/v2/catalog/object/O1" {
		t.Fatalf("path = %q, want /v2/catalog/object/O1", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("include_related_objects") != "true" {
		t.Errorf("include_related_objects = %q, want true", q.Get("include_related_objects"))
	}
}

// TestRawAPIEscapeHatch proves `api <method> <path>` injects auth + version and
// sends the raw body.
func TestRawAPIEscapeHatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"subscription":{"id":"S1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "api", "POST", "/v2/subscriptions", "--body", `{"idempotency_key":"k"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/subscriptions" {
		t.Fatalf("got %s %s, want POST /v2/subscriptions", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" || got.SquareVersion != squareVersion {
		t.Errorf("raw api did not inject auth/version: auth=%q version=%q", got.Auth, got.SquareVersion)
	}
}
