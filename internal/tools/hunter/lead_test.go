package hunter

import (
	"net/http"
	"testing"
)

func TestLeadList_Filters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "lead", "list",
		"--leads-list-id", "42", "--company", "Stripe", "--query", "eng", "--limit", "100")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/leads" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("leads_list_id") != "42" || q.Get("company") != "Stripe" || q.Get("query") != "eng" || q.Get("limit") != "100" {
		t.Errorf("query = %s", got.Query)
	}
}

func TestLeadGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":7}}`, &got)
	defer srv.Close()

	run(t, srv, "lead", "get", "--id", "7")
	if got.Method != http.MethodGet || got.Path != "/leads/7" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}

func TestLeadCreate_ExplicitFlagsPlusAttributes(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":9}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "lead", "create",
		"--email", "jane@stripe.com", "--first-name", "Jane", "--last-name", "Doe",
		"--company", "Stripe", "--leads-list-id", "42",
		"--attributes", `{"custom_attributes":{"tier":"gold"},"source":"api"}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/leads" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "jane@stripe.com" || body["first_name"] != "Jane" || body["last_name"] != "Doe" {
		t.Errorf("body = %v", body)
	}
	if body["leads_list_id"] != "42" || body["company"] != "Stripe" {
		t.Errorf("body = %v", body)
	}
	if body["source"] != "api" {
		t.Errorf("attributes not merged: %v", body)
	}
	if _, ok := body["custom_attributes"]; !ok {
		t.Errorf("custom_attributes not merged: %v", body)
	}
}

func TestLeadCreate_ExplicitFlagOverridesAttribute(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "lead", "create", "--email", "explicit@x.com",
		"--attributes", `{"email":"from-attr@x.com","company":"AttrCo"}`)
	body := decodeBody(t, got.Body)
	if body["email"] != "explicit@x.com" {
		t.Errorf("email = %v, want explicit flag to win", body["email"])
	}
	if body["company"] != "AttrCo" {
		t.Errorf("company = %v, want attribute passthrough", body["company"])
	}
}

func TestLeadUpdate_PutWithID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":9}}`, &got)
	defer srv.Close()

	run(t, srv, "lead", "update", "--id", "9", "--position", "CTO")
	if got.Method != http.MethodPut || got.Path != "/leads/9" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["position"] != "CTO" {
		t.Errorf("position = %v", body["position"])
	}
	if _, ok := body["email"]; ok {
		t.Errorf("unset email should be omitted from update body: %v", body)
	}
}

func TestLeadDelete_EmptyBodyReceipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "lead", "delete", "--id", "9")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/leads/9" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if stdout == "" {
		t.Error("want a deletion receipt even on empty 204 body")
	}
}
