package hunter

import (
	"net/http"
	"testing"
)

func TestDiscover_QueryAndFiltersMergedIntoBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "discover",
		"--query", "SaaS companies in France",
		"--filters", `{"headcount":"50-100","industry":["saas"]}`,
		"--limit", "25")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/discover" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["query"] != "SaaS companies in France" {
		t.Errorf("query = %v", body["query"])
	}
	if body["headcount"] != "50-100" {
		t.Errorf("headcount filter not merged: %v", body)
	}
	if _, ok := body["industry"]; !ok {
		t.Errorf("industry filter not merged: %v", body)
	}
	// limit is sent as a JSON number.
	if body["limit"] != float64(25) {
		t.Errorf("limit = %v (want numeric 25)", body["limit"])
	}
	if got.APIKey != "key-123" {
		t.Errorf("X-API-KEY = %q", got.APIKey)
	}
}

func TestDiscover_ExplicitQueryWinsOverFilterKey(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "discover", "--query", "explicit", "--filters", `{"query":"from-filter"}`)
	body := decodeBody(t, got.Body)
	if body["query"] != "explicit" {
		t.Errorf("query = %v, want explicit flag to override filter key", body["query"])
	}
}
