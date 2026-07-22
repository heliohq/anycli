package sage

import (
	"strings"
	"testing"
)

// TestPaginationParams proves --page / --items-per-page map to Sage's page /
// items_per_page query params.
func TestPaginationParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /contacts": {status: 200, body: `{"$items":[]}`},
	})
	defer srv.Close()

	_, errOut, res := run(t, srv, "contact", "list", "--page", "2", "--items-per-page", "50")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/contacts")
	if req == nil {
		t.Fatal("no GET /contacts recorded")
	}
	if got := req.Query.Get("page"); got != "2" {
		t.Errorf("page = %q, want 2", got)
	}
	if got := req.Query.Get("items_per_page"); got != "50" {
		t.Errorf("items_per_page = %q, want 50", got)
	}
}

// TestPaginationOmittedWhenUnset proves no paging params are sent when the flags
// are unset (so Sage applies its own defaults).
func TestPaginationOmittedWhenUnset(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /ledger_accounts": {status: 200, body: `{"$items":[]}`},
	})
	defer srv.Close()

	_, _, res := run(t, srv, "ledger-account", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	req := findReq(reqs, "GET", "/ledger_accounts")
	if req == nil {
		t.Fatal("no GET /ledger_accounts recorded")
	}
	if len(req.Query) != 0 {
		t.Errorf("unset paging must send no query params, got %v", req.Query)
	}
}

// TestGetEscapesID proves get builds <path>/<id> and URL-escapes the id.
func TestGetEscapesID(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /sales_invoices/inv 1": {status: 200, body: `{"id":"inv 1"}`},
	})
	defer srv.Close()

	_, errOut, res := run(t, srv, "sales-invoice", "get", "inv 1")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	if findReq(reqs, "GET", "/sales_invoices/inv 1") == nil {
		t.Fatalf("no GET /sales_invoices/inv 1 recorded; got %+v", reqs)
	}
}

// TestCreatePostsVerbatimBody proves create POSTs the --body JSON verbatim with
// a JSON Content-Type, and honors --business.
func TestCreatePostsVerbatimBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /contacts": {status: 201, body: `{"id":"c-new"}`},
	})
	defer srv.Close()

	body := `{"contact":{"name":"Acme","contact_type_ids":["CUSTOMER"]}}`
	_, errOut, res := run(t, srv, "contact", "create", "--business", "b9", "--body", body)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/contacts")
	if req == nil {
		t.Fatal("no POST /contacts recorded")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	if req.Business != "b9" {
		t.Errorf("X-Business = %q, want b9", req.Business)
	}
	m := bodyMap(t, req.Body)
	contact, ok := m["contact"].(map[string]any)
	if !ok {
		t.Fatalf("body did not carry the contact envelope verbatim: %s", req.Body)
	}
	if contact["name"] != "Acme" {
		t.Errorf("contact.name = %v, want Acme", contact["name"])
	}
}

// TestContactPaymentCreate proves the payments endpoint is /contact_payments
// (the documented v3.1 payment path), POSTing the contact_payment envelope.
func TestContactPaymentCreate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /contact_payments": {status: 201, body: `{"id":"p1"}`},
	})
	defer srv.Close()

	body := `{"contact_payment":{"transaction_type_id":"CUSTOMER_RECEIPT","total_amount":240}}`
	_, errOut, res := run(t, srv, "contact-payment", "create", "--body", body)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/contact_payments")
	if req == nil {
		t.Fatalf("no POST /contact_payments recorded; got %+v", reqs)
	}
	m := bodyMap(t, req.Body)
	if _, ok := m["contact_payment"]; !ok {
		t.Errorf("body did not carry the contact_payment envelope: %s", req.Body)
	}
}

// TestInvalidCreateBodyExitsTwo proves a malformed --body is a usage error that
// never reaches the API.
func TestInvalidCreateBodyExitsTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, errOut, res := run(t, srv, "contact", "create", "--body", "{not json")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (stderr=%q)", res.ExitCode, errOut)
	}
	if len(reqs) != 0 {
		t.Errorf("invalid body must not reach the API; got %d requests", len(reqs))
	}
}

// TestFetchPassthroughGet proves the generic fetch reaches an arbitrary path on
// the Bearer + X-Business path, normalizing a slashless --path.
func TestFetchPassthroughGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /addresses": {status: 200, body: `{"$items":[]}`},
	})
	defer srv.Close()

	_, errOut, res := run(t, srv, "fetch", "--path", "addresses", "--business", "bz")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "GET", "/addresses")
	if req == nil {
		t.Fatal("no GET /addresses recorded (slashless path not normalized?)")
	}
	if req.Business != "bz" {
		t.Errorf("X-Business = %q, want bz", req.Business)
	}
}

// TestFetchPassthroughPostBody proves fetch --method POST --body sends the body.
func TestFetchPassthroughPostBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /journals": {status: 201, body: `{"id":"j1"}`},
	})
	defer srv.Close()

	_, errOut, res := run(t, srv, "fetch", "--method", "post", "--path", "/journals", "--body", `{"journal":{"x":1}}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", res.ExitCode, errOut)
	}
	req := findReq(reqs, "POST", "/journals")
	if req == nil {
		t.Fatal("no POST /journals recorded")
	}
	if !strings.Contains(string(req.Body), `"journal"`) {
		t.Errorf("body = %s, want the journal envelope", req.Body)
	}
}

// TestFetchRejectsBadMethod proves an unsupported --method is a usage error.
func TestFetchRejectsBadMethod(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, res := run(t, srv, "fetch", "--method", "TRACE", "--path", "/x")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("bad method must not reach the API; got %d requests", len(reqs))
	}
}
