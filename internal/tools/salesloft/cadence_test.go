package salesloft

import (
	"net/http"
	"testing"
)

func TestCadenceList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/cadences": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "cadence", "list", "--per-page", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/cadences")
	if req == nil || req.Query.Get("per_page") != "10" {
		t.Fatalf("expected GET /v2/cadences with per_page=10, got %+v", req)
	}
}

func TestCadenceGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/cadences/3": {status: http.StatusOK, body: `{"data":{"id":3}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "cadence", "get", "--id", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/cadences/3") == nil {
		t.Fatal("expected GET /v2/cadences/3")
	}
}

func TestCadenceAddPerson(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/cadence_memberships": {status: http.StatusOK, body: `{"data":{"id":9}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "cadence", "add-person",
		"--person-id", "11", "--cadence-id", "22", "--user-id", "33")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPost, "/v2/cadence_memberships")
	if req == nil {
		t.Fatal("expected POST /v2/cadence_memberships")
	}
	body := bodyMap(t, req.Body)
	if body["person_id"] != float64(11) || body["cadence_id"] != float64(22) || body["user_id"] != float64(33) {
		t.Errorf("body = %v, want numeric person/cadence/user ids", body)
	}
}

func TestCadenceAddPerson_UserIDOptional(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/cadence_memberships": {status: http.StatusOK, body: `{"data":{"id":9}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "cadence", "add-person", "--person-id", "11", "--cadence-id", "22")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := bodyMap(t, findReq(reqs, http.MethodPost, "/v2/cadence_memberships").Body)
	if _, ok := body["user_id"]; ok {
		t.Errorf("user_id should be omitted when unset; body = %v", body)
	}
}

func TestCadenceMemberships_Filters(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/cadence_memberships": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "cadence", "memberships", "--person-id", "11", "--cadence-id", "22")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/cadence_memberships")
	if req == nil {
		t.Fatal("expected GET /v2/cadence_memberships")
	}
	if req.Query.Get("person_id[]") != "11" || req.Query.Get("cadence_id[]") != "22" {
		t.Errorf("membership filters = %v", req.Query)
	}
}
