package instantly

import (
	"net/http"
	"testing"
)

func TestAPIEscapeHatchBareResourcePath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "api", "GET", "subsequences", "--query", "limit=5")
	if exit != 0 {
		t.Fatalf("exit = %d stderr=%s", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/subsequences" {
		t.Fatalf("got %s %s, want GET /subsequences", got.Method, got.Path)
	}
	if parseQuery(t, got.Query).Get("limit") != "5" {
		t.Fatalf("query = %s", got.Query)
	}
	if got.Auth != "Bearer key-123" {
		t.Fatalf("auth = %q", got.Auth)
	}
}

func TestAPIEscapeHatchStripsApiV2Prefix(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	run(t, srv, "api", "GET", "/api/v2/campaigns")
	if got.Path != "/campaigns" {
		t.Fatalf("path = %s, want /campaigns (redundant /api/v2 stripped)", got.Path)
	}
}

func TestAPIEscapeHatchPOSTWithData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"x"}`, &got)
	defer srv.Close()

	run(t, srv, "api", "POST", "custom-tags", "--data", `{"label":"vip"}`)
	if got.Method != http.MethodPost || got.Path != "/custom-tags" {
		t.Fatalf("got %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["label"] != "vip" {
		t.Fatalf("body = %v", body)
	}
}

func TestAPIEscapeHatchBadQueryIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "api", "GET", "campaigns", "--query", "novalue")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for malformed --query", exit)
	}
}

func TestAPIEscapeHatchRequiresTwoArgs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "api", "GET")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for missing path arg", exit)
	}
}
