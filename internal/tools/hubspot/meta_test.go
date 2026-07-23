package hubspot

import (
	"net/http"
	"net/url"
	"testing"
)

func TestOwnerListFiltersByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "owner", "list", "--email", "rep@acme.com")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/crm/v3/owners" {
		t.Fatalf("path = %s", got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("email") != "rep@acme.com" {
		t.Fatalf("email = %q", q.Get("email"))
	}
}

func TestOwnerGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"5"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "owner", "get", "5")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/crm/v3/owners/5" {
		t.Fatalf("path = %s", got.Path)
	}
}

func TestPipelineList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "pipeline", "list", "deals")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/crm/v3/pipelines/deals" {
		t.Fatalf("path = %s", got.Path)
	}
}

func TestPropertyListAndGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "property", "list", "contacts"); exit != 0 {
		t.Fatalf("list exit = %d", exit)
	}
	if got.Path != "/crm/v3/properties/contacts" {
		t.Fatalf("list path = %s", got.Path)
	}

	if exit, _, _ := run(t, srv, "property", "get", "deals", "dealstage"); exit != 0 {
		t.Fatalf("get exit = %d", exit)
	}
	if got.Path != "/crm/v3/properties/deals/dealstage" {
		t.Fatalf("get path = %s", got.Path)
	}
}

func TestAssocCreateUsesV4DefaultPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"status":"COMPLETE"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "assoc", "create", "contact", "1", "company", "2")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodPut {
		t.Fatalf("method = %s", got.Method)
	}
	if got.Path != "/crm/v4/objects/contact/1/associations/default/company/2" {
		t.Fatalf("path = %s", got.Path)
	}
}

func TestAssocListPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "assoc", "list", "deal", "42", "contact")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodGet || got.Path != "/crm/v4/objects/deal/42/associations/contact" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
}

func TestAssocDeletePath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "assoc", "delete", "contact", "1", "company", "2")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodDelete || got.Path != "/crm/v4/objects/contact/1/associations/company/2" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
}
