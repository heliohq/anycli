package salesloft

import (
	"net/http"
	"testing"
)

func TestAccountList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/accounts": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "account", "list", "--updated-since", "2026-02-02T00:00:00Z")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/accounts")
	if req == nil || req.Query.Get("updated_at[gte]") != "2026-02-02T00:00:00Z" {
		t.Fatalf("expected GET /v2/accounts with updated_at[gte], got %+v", req)
	}
}

func TestAccountGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/accounts/12": {status: http.StatusOK, body: `{"data":{"id":12}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "account", "get", "--id", "12")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/accounts/12") == nil {
		t.Fatal("expected GET /v2/accounts/12")
	}
}

func TestAccountCreate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/accounts": {status: http.StatusOK, body: `{"data":{"id":1}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "account", "create",
		"--name", "Acme", "--domain", "acme.com", "--body", `{"industry":"Software"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPost, "/v2/accounts")
	if req == nil {
		t.Fatal("expected POST /v2/accounts")
	}
	body := bodyMap(t, req.Body)
	if body["name"] != "Acme" || body["domain"] != "acme.com" {
		t.Errorf("account fields = %v", body)
	}
	if body["industry"] != "Software" {
		t.Errorf("industry (from --body) = %v", body["industry"])
	}
}

func TestAccountUpdate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /v2/accounts/4": {status: http.StatusOK, body: `{"data":{"id":4}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "account", "update", "--id", "4", "--name", "Acme Inc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPut, "/v2/accounts/4")
	if req == nil || bodyMap(t, req.Body)["name"] != "Acme Inc" {
		t.Fatalf("expected PUT /v2/accounts/4 with name, got %+v", req)
	}
}
