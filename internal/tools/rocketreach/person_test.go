package rocketreach

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPersonLookup_ByNameAndEmployer(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":807344,"status":"searching","name":"Jane Doe"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "person", "lookup", "--name", "Jane Doe", "--current-employer", "Acme")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/api/v2/person/lookup" {
		t.Errorf("request = %s %s, want GET /api/v2/person/lookup", got.Method, got.Path)
	}
	assertAPIKey(t, got)
	if got.Query.Get("name") != "Jane Doe" || got.Query.Get("current_employer") != "Acme" {
		t.Errorf("query = %v, want name+current_employer", got.Query)
	}
	// The async status must surface so the agent knows whether to poll next.
	if !strings.Contains(stdout, `"status":"searching"`) {
		t.Errorf("stdout = %q, want the async status surfaced", stdout)
	}
}

func TestPersonLookup_ByLinkedIn(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":1,"status":"complete"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "lookup", "--linkedin-url", "https://linkedin.com/in/janedoe")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Query.Get("linkedin_url") != "https://linkedin.com/in/janedoe" {
		t.Errorf("query = %v, want linkedin_url", got.Query)
	}
}

func TestPersonLookup_ByID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":807344,"status":"complete"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "lookup", "--id", "807344")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Query.Get("id") != "807344" {
		t.Errorf("query = %v, want id", got.Query)
	}
}

func TestPersonLookup_NoIdentifier_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "person", "lookup")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing identifier", code)
	}
	if !strings.Contains(stderr, "provide") {
		t.Errorf("stderr = %q, want a usage error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

func TestPersonStatus_ByIDs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":807344,"status":"complete"}]`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "person", "status", "--ids", "807344,807345")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/api/v2/person/checkStatus" {
		t.Errorf("request = %s %s, want GET /api/v2/person/checkStatus", got.Method, got.Path)
	}
	if got.Query.Get("ids") != "807344,807345" {
		t.Errorf("query ids = %q, want 807344,807345", got.Query.Get("ids"))
	}
	if !strings.Contains(stdout, `"status":"complete"`) {
		t.Errorf("stdout = %q, want the status passthrough", stdout)
	}
}

func TestPersonStatus_MissingIDs_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "status")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for missing --ids", code)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

func TestPersonSearch_BuildsQueryBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"profiles":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "search",
		"--name", "Jane Doe", "--current-employer", "Acme", "--title", "VP", "--page-size", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v2/person/search" {
		t.Errorf("request = %s %s, want POST /api/v2/person/search", got.Method, got.Path)
	}
	assertAPIKey(t, got)
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ContentType)
	}
	body := bodyMap(t, got.Body)
	if body["page_size"].(float64) != 10 {
		t.Errorf("page_size = %v, want 10", body["page_size"])
	}
	query, ok := body["query"].(map[string]any)
	if !ok {
		t.Fatalf("body has no query object: %v", body)
	}
	name, _ := query["name"].([]any)
	if len(name) != 1 || name[0] != "Jane Doe" {
		t.Errorf("query.name = %v, want [\"Jane Doe\"]", query["name"])
	}
	title, _ := query["current_title"].([]any)
	if len(title) != 1 || title[0] != "VP" {
		t.Errorf("query.current_title = %v, want [\"VP\"]", query["current_title"])
	}
	emp, _ := query["current_employer"].([]any)
	if len(emp) != 1 || emp[0] != "Acme" {
		t.Errorf("query.current_employer = %v, want [\"Acme\"]", query["current_employer"])
	}
}

func TestPersonSearch_JSONQueryEscapeHatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"profiles":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "search",
		"--json-query", `{"keyword":["growth"],"location":["NYC"]}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, got.Body)
	query := body["query"].(map[string]any)
	kw, _ := query["keyword"].([]any)
	if len(kw) != 1 || kw[0] != "growth" {
		t.Errorf("query.keyword = %v, want [\"growth\"] from --json-query", query["keyword"])
	}
}

func TestPersonSearch_BadJSONQuery_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "person", "search", "--json-query", "{not json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for invalid JSON", code)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want the invalid-JSON usage error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent for bad JSON, got %s", got.Path)
	}
}

func TestPersonSearch_NoFilters_Usage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "search")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 when no filter is given", code)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

// The --json flag is accepted globally and must not be treated as a filter.
func TestPersonLookup_JSONFlagAccepted(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":1,"status":"complete"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "person", "lookup", "--id", "1", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &m); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout)
	}
}
