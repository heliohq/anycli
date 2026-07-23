package gumroad

import (
	"testing"
)

func TestSaleListWithFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"sales":[],"next_page_key":"k2"}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "sale", "list",
		"--after", "2026-01-01", "--before", "2026-02-01",
		"--email", "b@x.io", "--product-id", "p1", "--page-key", "k1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/v2/sales" {
		t.Fatalf("request = %s %s, want GET /v2/sales", got.Method, got.Path)
	}
	q, _ := parseQ(got.Query)
	for k, want := range map[string]string{
		"after": "2026-01-01", "before": "2026-02-01",
		"email": "b@x.io", "product_id": "p1", "page_key": "k1",
	} {
		if q.Get(k) != want {
			t.Fatalf("query[%s] = %q, want %q (raw=%s)", k, q.Get(k), want, got.Query)
		}
	}
	// Passthrough preserves the pagination cursor.
	v := decodeJSON(t, stdout).(map[string]any)
	if v["next_page_key"] != "k2" {
		t.Fatalf("next_page_key dropped: %s", stdout)
	}
}

func TestSaleGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"sale":{"id":"s1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "sale", "get", "--id", "s1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/v2/sales/s1" {
		t.Fatalf("request = %s %s, want GET /v2/sales/s1", got.Method, got.Path)
	}
}

func TestSaleMarkShipped(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"sale":{"id":"s1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "sale", "mark-shipped", "--id", "s1", "--tracking-url", "https://track/1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "PUT" || got.Path != "/v2/sales/s1/mark_as_shipped" {
		t.Fatalf("request = %s %s, want PUT /v2/sales/s1/mark_as_shipped", got.Method, got.Path)
	}
	if got.Form.Get("tracking_url") != "https://track/1" {
		t.Fatalf("form tracking_url = %q, want https://track/1 (body=%s)", got.Form.Get("tracking_url"), got.Body)
	}
}

func TestSaleRefund(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"sale":{"id":"s1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "sale", "refund", "--id", "s1", "--amount-cents", "500")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "PUT" || got.Path != "/v2/sales/s1/refund" {
		t.Fatalf("request = %s %s, want PUT /v2/sales/s1/refund", got.Method, got.Path)
	}
	if got.Form.Get("amount_cents") != "500" {
		t.Fatalf("form amount_cents = %q, want 500 (body=%s)", got.Form.Get("amount_cents"), got.Body)
	}
}

func TestSaleRefundOmitsAmountWhenUnset(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"sale":{"id":"s1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "sale", "refund", "--id", "s1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if _, ok := got.Form["amount_cents"]; ok {
		t.Fatalf("amount_cents should be absent for a full refund (body=%s)", got.Body)
	}
}

func TestSubscriberList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"subscribers":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "subscriber", "list", "--product-id", "p1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/v2/products/p1/subscribers" {
		t.Fatalf("request = %s %s, want GET /v2/products/p1/subscribers", got.Method, got.Path)
	}
}

func TestSubscriberGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"subscriber":{"id":"sub1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "subscriber", "get", "--id", "sub1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/v2/subscribers/sub1" {
		t.Fatalf("request = %s %s, want GET /v2/subscribers/sub1", got.Method, got.Path)
	}
}
