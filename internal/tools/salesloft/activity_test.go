package salesloft

import (
	"net/http"
	"testing"
)

func TestActivityList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/activity_histories": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "activity", "list", "--per-page", "20")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	req := findReq(reqs, http.MethodGet, "/v2/activity_histories")
	if req == nil || req.Query.Get("per_page") != "20" {
		t.Fatalf("expected GET /v2/activity_histories with per_page=20, got %+v", req)
	}
}

func TestEmailList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/activities/emails": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/activities/emails") == nil {
		t.Fatal("expected GET /v2/activities/emails")
	}
}

func TestEmailGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/activities/emails/7": {status: http.StatusOK, body: `{"data":{"id":7}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "get", "--id", "7")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/activities/emails/7") == nil {
		t.Fatal("expected GET /v2/activities/emails/7")
	}
}

func TestCallList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/activities/calls": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "call", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/activities/calls") == nil {
		t.Fatal("expected GET /v2/activities/calls")
	}
}
