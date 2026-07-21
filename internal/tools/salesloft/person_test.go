package salesloft

import (
	"net/http"
	"testing"
)

func TestPersonList_SharedFlags(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/people": {status: http.StatusOK, body: `{"data":[],"metadata":{"paging":{"current_page":1}}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "list",
		"--page", "2", "--per-page", "50",
		"--sort-by", "updated_at", "--sort-direction", "DESC",
		"--updated-since", "2026-01-01T00:00:00Z",
		"--email", "a@b.com", "--email", "c@d.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/people")
	if req == nil {
		t.Fatal("expected GET /v2/people")
	}
	q := req.Query
	if q.Get("page") != "2" || q.Get("per_page") != "50" {
		t.Errorf("paging query = %v", q)
	}
	if q.Get("sort_by") != "updated_at" || q.Get("sort_direction") != "DESC" {
		t.Errorf("sort query = %v", q)
	}
	if q.Get("updated_at[gte]") != "2026-01-01T00:00:00Z" {
		t.Errorf("updated_at[gte] = %q", q.Get("updated_at[gte]"))
	}
	emails := q["email_addresses[]"]
	if len(emails) != 2 {
		t.Errorf("email_addresses[] = %v, want two values", emails)
	}
}

func TestPersonList_PerPageOverMaxIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "list", "--per-page", "500")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/people") != nil {
		t.Error("should not call API when --per-page exceeds 100")
	}
}

func TestPersonList_GenericFilterPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/people": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "list", "--filter", "person_stage_id[]=7")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/people")
	if req.Query.Get("person_stage_id[]") != "7" {
		t.Errorf("filter passthrough = %v", req.Query)
	}
}

func TestPersonGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/people/42": {status: http.StatusOK, body: `{"data":{"id":42}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "get", "--id", "42")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/people/42") == nil {
		t.Fatal("expected GET /v2/people/42")
	}
}

func TestPersonCreate_NamedFields(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/people": {status: http.StatusOK, body: `{"data":{"id":1}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "create",
		"--email", "new@example.com", "--first-name", "Ada", "--last-name", "Lovelace",
		"--title", "CTO", "--account-id", "77")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPost, "/v2/people")
	if req == nil {
		t.Fatal("expected POST /v2/people")
	}
	if req.ContentType != "application/json" {
		t.Errorf("Content-Type = %q", req.ContentType)
	}
	body := bodyMap(t, req.Body)
	if body["email_address"] != "new@example.com" {
		t.Errorf("email_address = %v", body["email_address"])
	}
	if body["first_name"] != "Ada" || body["last_name"] != "Lovelace" || body["title"] != "CTO" {
		t.Errorf("name/title fields = %v", body)
	}
	// account_id is an integer field.
	if body["account_id"] != float64(77) {
		t.Errorf("account_id = %v, want numeric 77", body["account_id"])
	}
}

func TestPersonUpdate_BodyOverride(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /v2/people/5": {status: http.StatusOK, body: `{"data":{"id":5}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "person", "update", "--id", "5",
		"--title", "VP", "--body", `{"custom_fields":{"Region":"EU"},"title":"SVP"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPut, "/v2/people/5")
	if req == nil {
		t.Fatal("expected PUT /v2/people/5")
	}
	body := bodyMap(t, req.Body)
	// --body keys override named-flag keys.
	if body["title"] != "SVP" {
		t.Errorf("title = %v, want --body override SVP", body["title"])
	}
	if _, ok := body["custom_fields"]; !ok {
		t.Errorf("custom_fields missing; body = %v", body)
	}
}

func TestPersonCreate_InvalidBodyJSONIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, stderr := run(t, srv, "person", "create", "--email", "x@y.com", "--body", `{not json`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if findReq(reqs, http.MethodPost, "/v2/people") != nil {
		t.Error("should not call API on invalid --body JSON")
	}
	if stderr == "" {
		t.Error("expected a validation error on stderr")
	}
}
