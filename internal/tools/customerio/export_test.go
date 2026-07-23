package customerio

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestExportPeople_FiltersBodyKey(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"export":{"id":"e1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "export", "people", "--filters", `{"segment":{"id":3}}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/exports/customers" {
		t.Errorf("got %s %s, want POST /v1/exports/customers", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	// POST /v1/exports/customers requires the audience filter under `filters`
	// (plural); `filter` singular belongs to the customer-search endpoint.
	filters, ok := body["filters"].(map[string]any)
	if !ok {
		t.Fatalf("body.filters missing/wrong type: %v", body)
	}
	seg, ok := filters["segment"].(map[string]any)
	if !ok || seg["id"].(float64) != 3 {
		t.Errorf("filters.segment = %v, want {id:3}", filters["segment"])
	}
	if _, wrong := body["filter"]; wrong {
		t.Errorf("unexpected singular `filter` key in body: %v", body)
	}
}

func TestExportPeople_RequiresFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// `filters` is required by the spec; omitting it must be a usage error
	// rather than a full-workspace export.
	if exit, _, _ := run(t, srv, "export", "people"); exit != 2 {
		t.Errorf("missing-filters exit = %d, want 2", exit)
	}
}

func TestExportGet_MetadataOnly(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"export":{"id":"e1","status":"in_progress"}}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "export", "get", "--id", "e1")
	if exit != 0 {
		t.Fatalf("exit != 0")
	}
	if got.Path != "/v1/exports/e1" {
		t.Errorf("path = %q, want /v1/exports/e1", got.Path)
	}
	if len(stdout) == 0 {
		t.Error("expected the export metadata on stdout")
	}
}

func TestExportGet_DownloadFollowsURLAndWritesFile(t *testing.T) {
	captured := map[string]capturedRequest{}
	dir := t.TempDir()
	out := filepath.Join(dir, "deliveries.csv")
	payload := "id,state\n1,delivered\n"

	// Real two-step contract: GET /v1/exports/{id}/download (bearer-authed)
	// returns {"url":"…"} — a signed link — and that link's /signed route
	// serves the file bytes without any App API bearer header.
	var srvURL string
	routes := map[string]routeHandler{}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()
	srvURL = srv.URL
	routes["/v1/exports/e1/download"] = routeHandler{status: http.StatusOK,
		response: `{"url":"` + srvURL + `/signed"}`}
	routes["/signed"] = routeHandler{status: http.StatusOK, response: payload}

	exit, stdout, stderr := run(t, srv, "export", "get", "--id", "e1", "--download", "--out", out)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	// The download-link request hits the dedicated endpoint with the bearer.
	if got := captured["/v1/exports/e1/download"]; got.Auth != "Bearer key-123" {
		t.Errorf("download-link Authorization = %q, want %q", got.Auth, "Bearer key-123")
	}
	// The signed download must NOT carry the App API bearer header.
	if got := captured["/signed"]; got.Auth != "" {
		t.Errorf("signed download Authorization = %q, want empty (pre-signed URL)", got.Auth)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(data) != payload {
		t.Errorf("file = %q, want %q", data, payload)
	}
	var receipt struct {
		OK    bool   `json:"ok"`
		Path  string `json:"path"`
		Bytes int64  `json:"bytes"`
	}
	if err := json.Unmarshal([]byte(stdout), &receipt); err != nil {
		t.Fatalf("stdout not a receipt: %v (%s)", err, stdout)
	}
	if !receipt.OK || receipt.Path != out || receipt.Bytes != int64(len(payload)) {
		t.Errorf("receipt = %+v, want ok/path/bytes for %d bytes", receipt, len(payload))
	}
}

func TestExportGet_DownloadRequiresOut(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"url":"https://x/y"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "export", "get", "--id", "e1", "--download")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 when --out missing with --download", exit)
	}
}

func TestExportGet_DownloadNoURLIsAPIError(t *testing.T) {
	var got capturedRequest
	// A not-yet-ready export: the /download endpoint answers 200 with an empty
	// body (no signed url), which must surface as a runtime apiError.
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "export", "get", "--id", "e1", "--download", "--out", filepath.Join(t.TempDir(), "f"))
	if exit != 1 {
		t.Errorf("exit = %d, want 1 when no signed url yet; stderr=%q", exit, stderr)
	}
	if got.Path != "/v1/exports/e1/download" {
		t.Errorf("path = %q, want /v1/exports/e1/download", got.Path)
	}
}
