package microsoftonedrive

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/root/search(q='quarterly report')": {http.StatusOK, `{"value":[{"id":"s1","name":"Q1.pdf","size":10,"file":{"mimeType":"application/pdf"}}]}`},
	})
	stdout := f.runOK(t, "search", "--query", "quarterly report")
	if !strings.Contains(stdout, "Q1.pdf") {
		t.Errorf("output = %q, want the search hit", stdout)
	}
	got := f.last(t, "GET", "/v1.0/me/drive/root/search(q='quarterly report')")
	if !strings.Contains(got.Query, "top=20") {
		t.Errorf("query = %q, want $top", got.Query)
	}
}

func TestDownload_SavesFile(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/items/id1":         {http.StatusOK, `{"id":"id1","name":"data.bin","size":5}`},
		"GET /v1.0/me/drive/items/id1/content": {http.StatusOK, "hello"},
	})
	dir := t.TempDir()
	stdout := f.runOK(t, "download", "id1", "--save", dir)
	dest := filepath.Join(dir, "data.bin")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("saved contents = %q, want the downloaded bytes", data)
	}
	if !strings.Contains(stdout, "data.bin") {
		t.Errorf("output = %q, want the saved path", stdout)
	}
}

func TestDownload_JSON(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/drive/items/id1":         {http.StatusOK, `{"id":"id1","name":"data.bin","size":3}`},
		"GET /v1.0/me/drive/items/id1/content": {http.StatusOK, "abc"},
	})
	dir := t.TempDir()
	stdout := f.runOK(t, "download", "id1", "--save", dir, "--json")
	if !strings.Contains(stdout, `"name":"data.bin"`) || !strings.Contains(stdout, `"size":3`) {
		t.Errorf("--json output = %q, want the saved-file record", stdout)
	}
}

func TestUpload_SmallDirectPut(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PUT /v1.0/me/drive/root:/notes.txt:/content": {http.StatusCreated, `{"id":"up1","name":"notes.txt"}`},
	})
	dir := t.TempDir()
	local := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(local, []byte("small body"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	stdout := f.runOK(t, "upload", local)
	if !strings.Contains(stdout, "uploaded notes.txt") {
		t.Errorf("output = %q, want uploaded confirmation", stdout)
	}
	got := f.last(t, "PUT", "/v1.0/me/drive/root:/notes.txt:/content")
	if got.ContentType != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", got.ContentType)
	}
	if string(got.Body) != "small body" {
		t.Errorf("uploaded body = %q, want file contents", got.Body)
	}
}

func TestUpload_ToFolderPathAndName(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PUT /v1.0/me/drive/root:/Docs/final.txt:/content": {http.StatusOK, `{"id":"up2","name":"final.txt"}`},
	})
	dir := t.TempDir()
	local := filepath.Join(dir, "draft.txt")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	f.runOK(t, "upload", local, "--to", "Docs", "--name", "final.txt")
	f.last(t, "PUT", "/v1.0/me/drive/root:/Docs/final.txt:/content")
}

func TestUpload_ToParentID(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PUT /v1.0/me/drive/items/parent9:/a.txt:/content": {http.StatusOK, `{"id":"up3","name":"a.txt"}`},
	})
	dir := t.TempDir()
	local := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(local, []byte("y"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	f.runOK(t, "upload", local, "--parent", "parent9")
	f.last(t, "PUT", "/v1.0/me/drive/items/parent9:/a.txt:/content")
}

func TestUpload_LargeUsesUploadSession(t *testing.T) {
	// A payload above simpleUploadMaxBytes forces the createUploadSession
	// branch; the chunk PUTs stream to the session uploadUrl.
	f := &fixture{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.requests = append(f.requests, recordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			ContentRng:  r.Header.Get("Content-Range"),
			Body:        body.Bytes(),
		})
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1.0/me/drive/root:/big.bin:/createUploadSession":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"uploadUrl":"` + srv.URL + `/upload-session"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/upload-session":
			// Final chunk (range ends at total-1) returns the DriveItem;
			// intermediate chunks return 202 Accepted.
			rng := r.Header.Get("Content-Range")
			if strings.Contains(rng, "/"+strconv.Itoa(len(payloadForTest))) && strings.Contains(rng, "-"+strconv.Itoa(len(payloadForTest)-1)+"/") {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":"big1","name":"big.bin"}`))
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"nextExpectedRanges":["x"]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	f.srv = srv
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	local := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(local, payloadForTest, 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	result, stdout, stderr := f.run(t, "upload", local, "--name", "big.bin")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %s)", result.ExitCode, stderr)
	}
	if !strings.Contains(stdout, "uploaded big.bin") {
		t.Errorf("output = %q, want uploaded confirmation", stdout)
	}

	f.last(t, "POST", "/v1.0/me/drive/root:/big.bin:/createUploadSession")

	var ranges []string
	for _, r := range f.requests {
		if r.Method == http.MethodPut && r.Path == "/upload-session" {
			ranges = append(ranges, r.ContentRng)
			if r.Auth != "" {
				t.Errorf("chunk PUT carried Authorization %q, want none on the session URL", r.Auth)
			}
		}
	}
	if len(ranges) != 2 {
		t.Fatalf("chunk PUTs = %d, want 2", len(ranges))
	}
	total := len(payloadForTest)
	wantFirst := "bytes 0-" + strconv.Itoa(uploadChunkSize-1) + "/" + strconv.Itoa(total)
	wantLast := "bytes " + strconv.Itoa(uploadChunkSize) + "-" + strconv.Itoa(total-1) + "/" + strconv.Itoa(total)
	if ranges[0] != wantFirst {
		t.Errorf("first Content-Range = %q, want %q", ranges[0], wantFirst)
	}
	if ranges[1] != wantLast {
		t.Errorf("last Content-Range = %q, want %q", ranges[1], wantLast)
	}
}

// payloadForTest spans exactly two upload-session chunks: it is above
// simpleUploadMaxBytes (forcing the session path) yet below 2*uploadChunkSize.
// It is shared by the large-upload test and its fake session server.
var payloadForTest = makePayload(simpleUploadMaxBytes + 7)

func makePayload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}
