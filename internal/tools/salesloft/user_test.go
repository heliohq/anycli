package salesloft

import (
	"net/http"
	"testing"
)

func TestUserList(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/users": {status: http.StatusOK, body: `{"data":[]}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "list", "--per-page", "25")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/users") == nil {
		t.Fatal("expected GET /v2/users")
	}
}

func TestUserGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/users/99": {status: http.StatusOK, body: `{"data":{"id":99}}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, "user", "get", "--id", "99")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/v2/users/99") == nil {
		t.Fatal("expected GET /v2/users/99")
	}
}
