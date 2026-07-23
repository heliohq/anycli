package salesloft

import (
	"net/http"
	"testing"
)

func TestNoteList_AssociationFilter(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/notes": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "note", "list",
		"--associated-with-type", "person", "--associated-with-id", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/notes")
	if req == nil {
		t.Fatal("expected GET /v2/notes")
	}
	if req.Query.Get("associated_with_type") != "person" || req.Query.Get("associated_with_id[]") != "5" {
		t.Errorf("note filters = %v", req.Query)
	}
}

func TestNoteCreate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/notes": {status: http.StatusOK, body: `{"data":{"id":1}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "note", "create",
		"--content", "Called, left VM", "--associated-with-type", "person", "--associated-with-id", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodPost, "/v2/notes")
	if req == nil {
		t.Fatal("expected POST /v2/notes")
	}
	body := bodyMap(t, req.Body)
	if body["content"] != "Called, left VM" {
		t.Errorf("content = %v", body["content"])
	}
	if body["associated_with_type"] != "person" {
		t.Errorf("associated_with_type = %v", body["associated_with_type"])
	}
	if body["associated_with_id"] != float64(5) {
		t.Errorf("associated_with_id = %v, want numeric 5", body["associated_with_id"])
	}
}
