package hubspot

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestContactGetProjectsProperties(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"1","properties":{"email":"a@b.com"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "contact", "get", "1", "--properties", "email,firstname")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/crm/v3/objects/contacts/1" {
		t.Fatalf("path = %s", got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if props := q["properties"]; len(props) != 2 || props[0] != "email" || props[1] != "firstname" {
		t.Fatalf("properties query = %v", props)
	}
}

func TestContactGetByEmailSetsIdProperty(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "contact", "get", "jane@acme.com", "--by-email")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/crm/v3/objects/contacts/jane@acme.com" {
		t.Fatalf("path = %s", got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("idProperty") != "email" {
		t.Fatalf("idProperty = %q", q.Get("idProperty"))
	}
}

func TestCompanyListPaginates(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "company", "list", "--limit", "10", "--after", "CURSOR", "--archived")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/crm/v3/objects/companies" {
		t.Fatalf("path = %s", got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("limit") != "10" || q.Get("after") != "CURSOR" || q.Get("archived") != "true" {
		t.Fatalf("query = %s", got.Query)
	}
}

func TestDealCreatePostsProperties(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"55"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "deal", "create", "--prop", "dealname=Acme Expansion", "--prop", "amount=5000")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/crm/v3/objects/deals" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	props, ok := body["properties"].(map[string]any)
	if !ok || props["dealname"] != "Acme Expansion" || props["amount"] != "5000" {
		t.Fatalf("body properties = %#v", body["properties"])
	}
}

func TestPropValueMayContainEquals(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"1"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "contact", "create", "--prop", "note=a=b=c")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	props := decodeBody(t, got.Body)["properties"].(map[string]any)
	if props["note"] != "a=b=c" {
		t.Fatalf("note = %v", props["note"])
	}
}

func TestBadPropIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "contact", "update", "1", "--prop", "novalue")
	if exit != 2 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "" {
		t.Fatal("bad --prop must not reach the API")
	}
	if !strings.Contains(stderr, "key=value") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestTicketUpdatePatches(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"7"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "ticket", "update", "7", "--prop", "hs_pipeline_stage=2")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != http.MethodPatch || got.Path != "/crm/v3/objects/tickets/7" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
}

func TestContactDeleteHandles204EmptyBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "contact", "delete", "1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodDelete {
		t.Fatalf("method = %s", got.Method)
	}
	if stdout != "" {
		t.Fatalf("204 delete should emit nothing, got %q", stdout)
	}
}

func TestContactSearchBuildsFilterGroups(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"contact", "search",
		"--query", "acme",
		"--filter", "email:CONTAINS_TOKEN:@acme.com",
		"--filter", "lifecyclestage:EQ:customer",
		"--sort", "createdate:desc",
		"--properties", "email,firstname",
		"--limit", "25",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/crm/v3/objects/contacts/search" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["query"] != "acme" {
		t.Fatalf("query = %v", body["query"])
	}
	groups, ok := body["filterGroups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("filterGroups = %#v", body["filterGroups"])
	}
	filters := groups[0].(map[string]any)["filters"].([]any)
	if len(filters) != 2 {
		t.Fatalf("filters = %#v", filters)
	}
	f0 := filters[0].(map[string]any)
	if f0["propertyName"] != "email" || f0["operator"] != "CONTAINS_TOKEN" || f0["value"] != "@acme.com" {
		t.Fatalf("filter[0] = %#v", f0)
	}
	sorts := body["sorts"].([]any)
	s0 := sorts[0].(map[string]any)
	if s0["propertyName"] != "createdate" || s0["direction"] != "DESCENDING" {
		t.Fatalf("sort[0] = %#v", s0)
	}
	props := body["properties"].([]any)
	if len(props) != 2 || props[0] != "email" {
		t.Fatalf("properties = %#v", props)
	}
	if body["limit"].(float64) != 25 {
		t.Fatalf("limit = %v", body["limit"])
	}
}

func TestSearchHasPropertyFilterOmitsValue(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "deal", "search", "--filter", "amount:HAS_PROPERTY")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	body := decodeBody(t, got.Body)
	filters := body["filterGroups"].([]any)[0].(map[string]any)["filters"].([]any)
	f0 := filters[0].(map[string]any)
	if f0["operator"] != "HAS_PROPERTY" {
		t.Fatalf("operator = %v", f0["operator"])
	}
	if _, present := f0["value"]; present {
		t.Fatalf("HAS_PROPERTY must omit value, got %#v", f0)
	}
}

func TestBadFilterIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "contact", "search", "--filter", "onlyproperty")
	if exit != 2 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "" {
		t.Fatal("bad --filter must not reach the API")
	}
}
