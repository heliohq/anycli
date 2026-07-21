package customerio

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

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

	// The metadata route carries a download_url pointing back at this server's
	// /signed route, which serves the file bytes.
	var srvURL string
	routes := map[string]routeHandler{}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()
	srvURL = srv.URL
	routes["/v1/exports/e1"] = routeHandler{status: http.StatusOK,
		response: `{"export":{"id":"e1","download_url":"` + srvURL + `/signed"}}`}
	routes["/signed"] = routeHandler{status: http.StatusOK, response: payload}

	exit, stdout, stderr := run(t, srv, "export", "get", "--id", "e1", "--download", "--out", out)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	// The signed download must NOT carry the App API bearer header.
	if got := captured["/signed"]; got.Auth != "" {
		t.Errorf("download Authorization = %q, want empty (pre-signed URL)", got.Auth)
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
	srv := newServer(t, http.StatusOK, `{"export":{"download_url":"https://x/y"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "export", "get", "--id", "e1", "--download")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 when --out missing with --download", exit)
	}
}

func TestExportGet_DownloadNoURLIsAPIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"export":{"id":"e1","status":"in_progress"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "export", "get", "--id", "e1", "--download", "--out", filepath.Join(t.TempDir(), "f"))
	if exit != 1 {
		t.Errorf("exit = %d, want 1 when no download_url yet; stderr=%q", exit, stderr)
	}
}
