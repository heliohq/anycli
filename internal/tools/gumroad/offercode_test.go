package gumroad

import "testing"

func TestOfferCodeList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"offer_codes":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "offer-code", "list", "--product-id", "p1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/v2/products/p1/offer_codes" {
		t.Fatalf("request = %s %s, want GET /v2/products/p1/offer_codes", got.Method, got.Path)
	}
}

func TestOfferCodeGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"offer_code":{"id":"o1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "offer-code", "get", "--product-id", "p1", "--id", "o1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/v2/products/p1/offer_codes/o1" {
		t.Fatalf("request = %s %s, want GET /v2/products/p1/offer_codes/o1", got.Method, got.Path)
	}
}

func TestOfferCodeCreateAbsolute(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"offer_code":{"id":"o1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "offer-code", "create",
		"--product-id", "p1", "--name", "SALE10", "--amount-off", "1000", "--max-purchase-count", "50")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "POST" || got.Path != "/v2/products/p1/offer_codes" {
		t.Fatalf("request = %s %s, want POST /v2/products/p1/offer_codes", got.Method, got.Path)
	}
	if got.Form.Get("name") != "SALE10" {
		t.Fatalf("form name = %q, want SALE10 (body=%s)", got.Form.Get("name"), got.Body)
	}
	if got.Form.Get("amount_off") != "1000" {
		t.Fatalf("form amount_off = %q, want 1000", got.Form.Get("amount_off"))
	}
	if got.Form.Get("offer_type") != "cents" {
		t.Fatalf("form offer_type = %q, want cents (absolute discount)", got.Form.Get("offer_type"))
	}
	if got.Form.Get("max_purchase_count") != "50" {
		t.Fatalf("form max_purchase_count = %q, want 50", got.Form.Get("max_purchase_count"))
	}
}

func TestOfferCodeCreatePercent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"offer_code":{"id":"o1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "offer-code", "create",
		"--product-id", "p1", "--name", "HALF", "--amount-off", "50", "--percent")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Form.Get("offer_type") != "percent" {
		t.Fatalf("form offer_type = %q, want percent", got.Form.Get("offer_type"))
	}
}

func TestOfferCodeUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"offer_code":{"id":"o1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "offer-code", "update",
		"--product-id", "p1", "--id", "o1", "--max-purchase-count", "5")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "PUT" || got.Path != "/v2/products/p1/offer_codes/o1" {
		t.Fatalf("request = %s %s, want PUT /v2/products/p1/offer_codes/o1", got.Method, got.Path)
	}
	if got.Form.Get("max_purchase_count") != "5" {
		t.Fatalf("form max_purchase_count = %q, want 5", got.Form.Get("max_purchase_count"))
	}
}

func TestOfferCodeDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "offer-code", "delete", "--product-id", "p1", "--id", "o1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "DELETE" || got.Path != "/v2/products/p1/offer_codes/o1" {
		t.Fatalf("request = %s %s, want DELETE /v2/products/p1/offer_codes/o1", got.Method, got.Path)
	}
}
