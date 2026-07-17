package bitly

import (
	"net/http"
	"testing"
)

func TestGroupList_OrganizationFilter(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"groups":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "group", "list", "--organization", "Oa1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/groups" {
		t.Errorf("request = %s %s, want GET /groups", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("organization_guid") != "Oa1" {
		t.Errorf("organization_guid = %q", q.Get("organization_guid"))
	}
}

func TestGroupGet_AutoResolve(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/user":           {status: http.StatusOK, response: `{"default_group_guid":"Bg-auto"}`},
		"/groups/Bg-auto": {status: http.StatusOK, response: `{"guid":"Bg-auto"}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "group", "get")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if _, ok := captured["/user"]; !ok {
		t.Fatal("expected GET /user for auto-resolution")
	}
	if _, ok := captured["/groups/Bg-auto"]; !ok {
		t.Fatalf("expected GET /groups/Bg-auto, captured = %v", captured)
	}
}

func TestGroupShortenCounts_AnalyticsNoSize(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "group", "shorten-counts", "--group", "Bg1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/groups/Bg1/shorten_counts" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("unit") != "day" || q.Get("units") != "-1" {
		t.Errorf("query = %q, want default unit window", got.Query)
	}
	if _, ok := q["size"]; ok {
		t.Errorf("size should be absent on shorten-counts, query = %q", got.Query)
	}
}

func TestGroupCountries_AnalyticsWithSize(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "group", "countries", "--group", "Bg1", "--size", "3")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/groups/Bg1/countries" {
		t.Errorf("path = %q", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("size") != "3" || q.Get("unit") != "day" {
		t.Errorf("query = %q", got.Query)
	}
}
