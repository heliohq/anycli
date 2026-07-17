package bitly

import (
	"net/http"
	"testing"
)

func TestAnalytics_DefaultUnitWindow(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"link_clicks":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "analytics", "clicks", "--bitlink", "bit.ly/2ab")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/bitlinks/bit.ly/2ab/clicks" {
		t.Errorf("request = %s %s, want GET /bitlinks/bit.ly/2ab/clicks", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("unit") != "day" {
		t.Errorf("unit = %q, want default day", q.Get("unit"))
	}
	if q.Get("units") != "-1" {
		t.Errorf("units = %q, want default -1", q.Get("units"))
	}
	if _, ok := q["unit_reference"]; ok {
		t.Errorf("unit_reference should be omitted when empty, query = %q", got.Query)
	}
	if _, ok := q["size"]; ok {
		t.Errorf("size should be absent on a totals endpoint, query = %q", got.Query)
	}
}

func TestAnalytics_CountriesWithSizeAndOverrides(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"metrics":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "analytics", "countries", "--bitlink", "bit.ly/2ab",
		"--unit", "month", "--units", "6", "--unit-reference", "2030-01-01T00:00:00+0000", "--size", "5")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/bitlinks/bit.ly/2ab/countries" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("unit") != "month" || q.Get("units") != "6" || q.Get("size") != "5" {
		t.Errorf("query = %q", got.Query)
	}
	if q.Get("unit_reference") != "2030-01-01T00:00:00+0000" {
		t.Errorf("unit_reference = %q", q.Get("unit_reference"))
	}
}

func TestAnalytics_EngagementsSummaryPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "analytics", "engagements-summary", "--bitlink", "bit.ly/2ab")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/bitlinks/bit.ly/2ab/engagements/summary" {
		t.Errorf("path = %q, want /bitlinks/bit.ly/2ab/engagements/summary", got.Path)
	}
	if _, ok := parseQuery(t, got.Query)["size"]; ok {
		t.Errorf("size should be absent on engagements-summary, query = %q", got.Query)
	}
}

func TestAnalytics_APIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"message":"NOT_FOUND"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "analytics", "clicks", "--bitlink", "bit.ly/missing")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if len(stderr) == 0 {
		t.Error("expected an error message on stderr")
	}
}
