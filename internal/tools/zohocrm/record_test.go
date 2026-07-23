package zohocrm

import (
	"net/http"
	"strings"
	"testing"
)

func TestRecordList(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /crm/v8/Leads": {status: 200, body: `{"data":[{"id":"1","Last_Name":"Doe"}],"info":{"more_records":false}}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "list", "--module", "Leads",
		"--fields", "Last_Name,Email", "--page", "2", "--per-page", "50",
		"--sort-by", "Modified_Time", "--sort-order", "desc")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/crm/v8/Leads")
	if req == nil {
		t.Fatal("no GET /crm/v8/Leads request recorded")
	}
	if req.Auth != "Zoho-oauthtoken test-token" {
		t.Errorf("auth header = %q, want Zoho-oauthtoken test-token", req.Auth)
	}
	if req.Query.Get("fields") != "Last_Name,Email" {
		t.Errorf("fields = %q, want Last_Name,Email", req.Query.Get("fields"))
	}
	if req.Query.Get("page") != "2" || req.Query.Get("per_page") != "50" {
		t.Errorf("page/per_page = %q/%q, want 2/50", req.Query.Get("page"), req.Query.Get("per_page"))
	}
	if req.Query.Get("sort_by") != "Modified_Time" || req.Query.Get("sort_order") != "desc" {
		t.Errorf("sort = %q/%q", req.Query.Get("sort_by"), req.Query.Get("sort_order"))
	}
	if !strings.Contains(got.stdout, `"Last_Name":"Doe"`) {
		t.Errorf("stdout missing provider JSON: %s", got.stdout)
	}
}

func TestRecordListRequiresFields(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "record", "list", "--module", "Leads")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", got.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made when --fields is missing, got %d", len(reqs))
	}
	if !strings.Contains(got.stderr, "field list") {
		t.Errorf("error should point at `field list`: %s", got.stderr)
	}
}

func TestRecordListPageAndPageTokenExclusive(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	got := run(t, srv, "record", "list", "--module", "Leads", "--fields", "id",
		"--page", "1", "--page-token", "abc")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", got.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call on mutually-exclusive flags, got %d", len(reqs))
	}
}

func TestRecordListPageToken(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/Contacts": {status: 200, body: `{"data":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "list", "--module", "Contacts", "--fields", "id", "--page-token", "TKN123")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/crm/v8/Contacts")
	if req.Query.Get("page_token") != "TKN123" {
		t.Errorf("page_token = %q, want TKN123", req.Query.Get("page_token"))
	}
}

func TestRecordListBadSortBy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	got := run(t, srv, "record", "list", "--module", "Leads", "--fields", "id", "--sort-by", "Name")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for bad --sort-by", got.result.ExitCode)
	}
}

func TestRecordGet(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/Deals/555": {status: 200, body: `{"data":[{"id":"555"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "get", "--module", "Deals", "--id", "555", "--fields", "Deal_Name,Amount")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/crm/v8/Deals/555")
	if req == nil {
		t.Fatal("no GET /crm/v8/Deals/555")
	}
	if req.Query.Get("fields") != "Deal_Name,Amount" {
		t.Errorf("fields = %q", req.Query.Get("fields"))
	}
}

func TestRecordCreateWrapsDataObject(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"POST /crm/v8/Leads": {status: 201, body: `{"data":[{"code":"SUCCESS","status":"success"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "create", "--module", "Leads",
		"--data", `{"Last_Name":"Doe","Company":"Acme"}`)
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodPost, "/crm/v8/Leads")
	if req.ContentType != "application/json" {
		t.Errorf("content-type = %q", req.ContentType)
	}
	m := bodyMap(t, req.Body)
	data, ok := m["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("body.data should be a 1-element array, got %v", m["data"])
	}
	if _, hasTrigger := m["trigger"]; hasTrigger {
		t.Errorf("trigger must be absent without --no-triggers, got %v", m["trigger"])
	}
	rec := data[0].(map[string]any)
	if rec["Last_Name"] != "Doe" {
		t.Errorf("record not preserved: %v", rec)
	}
}

func TestRecordCreateArrayAndNoTriggers(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"POST /crm/v8/Contacts": {status: 200, body: `{"data":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "create", "--module", "Contacts",
		"--data", `[{"Last_Name":"A"},{"Last_Name":"B"}]`, "--no-triggers")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodPost, "/crm/v8/Contacts")
	m := bodyMap(t, req.Body)
	if data, _ := m["data"].([]any); len(data) != 2 {
		t.Fatalf("body.data should have 2 records, got %v", m["data"])
	}
	trig, ok := m["trigger"].([]any)
	if !ok || len(trig) != 0 {
		t.Errorf("--no-triggers must set trigger:[], got %v", m["trigger"])
	}
}

func TestRecordCreateRequiresData(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	got := run(t, srv, "record", "create", "--module", "Leads")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing --data", got.result.ExitCode)
	}
}

func TestRecordCreateInvalidJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	got := run(t, srv, "record", "create", "--module", "Leads", "--data", `{not json`)
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for invalid JSON", got.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call on invalid JSON, got %d", len(reqs))
	}
}

func TestRecordUpdate(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"PUT /crm/v8/Deals/999": {status: 200, body: `{"data":[{"code":"SUCCESS"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "update", "--module", "Deals", "--id", "999",
		"--data", `{"Stage":"Closed Won"}`)
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodPut, "/crm/v8/Deals/999")
	m := bodyMap(t, req.Body)
	data := m["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["Stage"] != "Closed Won" {
		t.Errorf("update body wrong: %v", m)
	}
}

func TestRecordUpdateRejectsArray(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	got := run(t, srv, "record", "update", "--module", "Deals", "--id", "1", "--data", `[{"Stage":"X"}]`)
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (update takes a single object)", got.result.ExitCode)
	}
}

func TestRecordDeleteSingle(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"DELETE /crm/v8/Leads/42": {status: 200, body: `{"data":[{"code":"SUCCESS"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "delete", "--module", "Leads", "--id", "42")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	if findReq(reqs, http.MethodDelete, "/crm/v8/Leads/42") == nil {
		t.Fatal("no DELETE /crm/v8/Leads/42")
	}
}

func TestRecordDeleteBulk(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"DELETE /crm/v8/Leads": {status: 200, body: `{"data":[]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "delete", "--module", "Leads", "--ids", "1,2,3")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodDelete, "/crm/v8/Leads")
	if req.Query.Get("ids") != "1,2,3" {
		t.Errorf("ids = %q, want 1,2,3", req.Query.Get("ids"))
	}
}

func TestRecordDeleteRequiresExactlyOne(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	both := run(t, srv, "record", "delete", "--module", "Leads", "--id", "1", "--ids", "2,3")
	if both.result.ExitCode != 2 {
		t.Errorf("both --id and --ids: exit = %d, want 2", both.result.ExitCode)
	}
	neither := run(t, srv, "record", "delete", "--module", "Leads")
	if neither.result.ExitCode != 2 {
		t.Errorf("neither: exit = %d, want 2", neither.result.ExitCode)
	}
}

func TestRecordSearchByEmail(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/Contacts/search": {status: 200, body: `{"data":[{"id":"7"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "search", "--module", "Contacts", "--email", "jane@acme.com", "--fields", "Email")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/crm/v8/Contacts/search")
	if req.Query.Get("email") != "jane@acme.com" {
		t.Errorf("email = %q", req.Query.Get("email"))
	}
	if req.Query.Get("fields") != "Email" {
		t.Errorf("fields = %q", req.Query.Get("fields"))
	}
}

func TestRecordSearchRequiresExactlyOneSelector(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	none := run(t, srv, "record", "search", "--module", "Leads")
	if none.result.ExitCode != 2 {
		t.Errorf("no selector: exit = %d, want 2", none.result.ExitCode)
	}
	two := run(t, srv, "record", "search", "--module", "Leads", "--email", "a@b.com", "--word", "acme")
	if two.result.ExitCode != 2 {
		t.Errorf("two selectors: exit = %d, want 2", two.result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call on bad selector count, got %d", len(reqs))
	}
}

func TestRecordSearchEmptyResult204(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/Leads/search": {status: http.StatusNoContent}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "record", "search", "--module", "Leads", "--word", "nobody")
	if got.result.ExitCode != 0 {
		t.Fatalf("204 should be success (exit 0), got %d; stderr=%s", got.result.ExitCode, got.stderr)
	}
	if strings.TrimSpace(got.stdout) != "" {
		t.Errorf("204 should emit nothing, got %q", got.stdout)
	}
}
