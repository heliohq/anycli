package instantly

import (
	"net/http"
	"testing"
)

func TestLeadListIsPOSTWithBodyPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[],"next_starting_after":"n1"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "lead", "list", "--campaign", "c1", "--limit", "50", "--starting-after", "cur")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/leads/list" {
		t.Fatalf("got %s %s, want POST /leads/list", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["campaign"] != "c1" {
		t.Fatalf("campaign = %v", body["campaign"])
	}
	// numbers decode as float64
	if body["limit"].(float64) != 50 {
		t.Fatalf("limit = %v, want 50", body["limit"])
	}
	if body["starting_after"] != "cur" {
		t.Fatalf("starting_after = %v", body["starting_after"])
	}
}

func TestLeadListDataFlagMergedFlagsOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	run(t, srv, "lead", "list", "--data", `{"campaign":"old","filter":{"k":"v"}}`, "--campaign", "new")
	body := decodeBody(t, got.Body)
	if body["campaign"] != "new" {
		t.Fatalf("campaign = %v, want flag override 'new'", body["campaign"])
	}
	if _, ok := body["filter"]; !ok {
		t.Fatalf("filter from --data should be preserved: %v", body)
	}
}

func TestLeadCreateMapsConvenienceFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"l1"}`, &got)
	defer srv.Close()

	run(t, srv, "lead", "create", "--email", "a@b.com", "--campaign", "c1", "--first-name", "Ada", "--company-name", "Acme")
	if got.Method != http.MethodPost || got.Path != "/leads" {
		t.Fatalf("got %s %s, want POST /leads", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@b.com" || body["campaign"] != "c1" || body["first_name"] != "Ada" || body["company_name"] != "Acme" {
		t.Fatalf("body = %v", body)
	}
}

func TestLeadGetUpdateDelete(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		method string
		path   string
	}{
		{[]string{"lead", "get", "--id", "l1"}, http.MethodGet, "/leads/l1"},
		{[]string{"lead", "update", "--id", "l1", "--data", `{"first_name":"Bo"}`}, http.MethodPatch, "/leads/l1"},
		{[]string{"lead", "delete", "--id", "l1"}, http.MethodDelete, "/leads/l1"},
	} {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, `{}`, &got)
		exit, _, _ := run(t, srv, tc.args...)
		srv.Close()
		if exit != 0 {
			t.Fatalf("%v exit = %d", tc.args, exit)
		}
		if got.Method != tc.method || got.Path != tc.path {
			t.Fatalf("%v got %s %s, want %s %s", tc.args, got.Method, got.Path, tc.method, tc.path)
		}
	}
}

func TestLeadAddRequiresLeadsViaData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "lead", "add", "--campaign-id", "c1", "--data", `{"leads":[{"email":"a@b.com"}]}`)
	if got.Method != http.MethodPost || got.Path != "/leads/add" {
		t.Fatalf("got %s %s, want POST /leads/add", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["campaign_id"] != "c1" {
		t.Fatalf("campaign_id = %v", body["campaign_id"])
	}
	if _, ok := body["leads"]; !ok {
		t.Fatalf("leads array from --data should be present: %v", body)
	}
}

func TestLeadMoveMapsDestinations(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"job_id":"j1"}`, &got)
	defer srv.Close()

	run(t, srv, "lead", "move", "--to-campaign-id", "c2", "--data", `{"ids":["l1","l2"]}`)
	if got.Path != "/leads/move" {
		t.Fatalf("path = %s, want /leads/move", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["to_campaign_id"] != "c2" {
		t.Fatalf("to_campaign_id = %v", body["to_campaign_id"])
	}
}

func TestLeadUpdateInterestRequiredFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "lead", "update-interest", "--lead-email", "a@b.com", "--interest-value", "1", "--campaign-id", "c1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/leads/update-interest-status" {
		t.Fatalf("path = %s", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["lead_email"] != "a@b.com" || body["interest_value"].(float64) != 1 || body["campaign_id"] != "c1" {
		t.Fatalf("body = %v", body)
	}
}

func TestLeadUpdateInterestMissingRequiredIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "lead", "update-interest", "--lead-email", "a@b.com")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for missing --interest-value", exit)
	}
}
