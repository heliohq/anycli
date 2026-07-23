package zohocrm

import (
	"net/http"
	"strings"
	"testing"
)

func TestQueryCOQL(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"POST /crm/v8/coql": {status: 200, body: `{"data":[{"Last_Name":"Doe"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "query", "--coql", "select Last_Name from Leads where Email is not null limit 200")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodPost, "/crm/v8/coql")
	if req == nil {
		t.Fatal("no POST /crm/v8/coql")
	}
	m := bodyMap(t, req.Body)
	if m["select_query"] != "select Last_Name from Leads where Email is not null limit 200" {
		t.Errorf("select_query wrong: %v", m["select_query"])
	}
}

func TestQueryRequiresCoql(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	got := run(t, srv, "query")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing --coql", got.result.ExitCode)
	}
}

func TestNoteList(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/Deals/12/Notes": {status: 200, body: `{"data":[{"Note_Title":"x"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "note", "list", "--module", "Deals", "--id", "12")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	if findReq(reqs, http.MethodGet, "/crm/v8/Deals/12/Notes") == nil {
		t.Fatal("no GET /crm/v8/Deals/12/Notes")
	}
}

func TestNoteAdd(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"POST /crm/v8/Leads/34/Notes": {status: 201, body: `{"data":[{"code":"SUCCESS"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "note", "add", "--module", "Leads", "--id", "34",
		"--title", "Call summary", "--content", "Spoke about renewal")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodPost, "/crm/v8/Leads/34/Notes")
	m := bodyMap(t, req.Body)
	data := m["data"].([]any)
	note := data[0].(map[string]any)
	if note["Note_Title"] != "Call summary" || note["Note_Content"] != "Spoke about renewal" {
		t.Errorf("note body wrong: %v", note)
	}
}

func TestNoteAddRequiresTitleAndContent(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	got := run(t, srv, "note", "add", "--module", "Leads", "--id", "1", "--title", "x")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing --content", got.result.ExitCode)
	}
}

func TestModuleList(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/settings/modules": {status: 200, body: `{"modules":[{"api_name":"Leads"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "module", "list")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	if findReq(reqs, http.MethodGet, "/crm/v8/settings/modules") == nil {
		t.Fatal("no GET /crm/v8/settings/modules")
	}
}

func TestFieldList(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/settings/fields": {status: 200, body: `{"fields":[{"api_name":"Last_Name"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "field", "list", "--module", "Leads")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/crm/v8/settings/fields")
	if req.Query.Get("module") != "Leads" {
		t.Errorf("module param = %q, want Leads", req.Query.Get("module"))
	}
}

func TestFieldListRequiresModule(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	got := run(t, srv, "field", "list")
	if got.result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for missing --module", got.result.ExitCode)
	}
}

func TestUserListWithType(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/users": {status: 200, body: `{"users":[{"id":"1"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "user", "list", "--type", "ActiveUsers")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/crm/v8/users")
	if req.Query.Get("type") != "ActiveUsers" {
		t.Errorf("type = %q, want ActiveUsers", req.Query.Get("type"))
	}
}

func TestUserMe(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/users": {status: 200, body: `{"users":[{"id":"99","email":"me@acme.com"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "user", "me")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	req := findReq(reqs, http.MethodGet, "/crm/v8/users")
	if req.Query.Get("type") != "CurrentUser" {
		t.Errorf("type = %q, want CurrentUser", req.Query.Get("type"))
	}
	if !strings.Contains(got.stdout, "me@acme.com") {
		t.Errorf("stdout missing user: %s", got.stdout)
	}
}

func TestOrgGet(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{"GET /crm/v8/org": {status: 200, body: `{"org":[{"id":"o1"}]}`}}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	got := run(t, srv, "org", "get")
	if got.result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", got.result.ExitCode, got.stderr)
	}
	if findReq(reqs, http.MethodGet, "/crm/v8/org") == nil {
		t.Fatal("no GET /crm/v8/org")
	}
}
