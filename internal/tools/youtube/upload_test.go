package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// sessionPath is the path of the fake resumable-upload session URL the init
// response's Location header points at (absolute, on the fixture server).
const sessionPath = "/upload-session/abc123"

// uploadRecorded captures one request the fake resumable-upload server saw.
// harness_test.go's fixture cannot express a Location response header or
// record r.ContentLength, so uploads get their own fake.
type uploadRecorded struct {
	Query         url.Values
	Header        http.Header
	ContentLength int64
	Body          []byte
}

// uploadFixture is a fake resumable-upload server: an init POST that answers
// with an absolute Location pointing back at itself, and a session PUT.
type uploadFixture struct {
	srv  *httptest.Server
	init *uploadRecorded
	put  *uploadRecorded

	initStatus int
	initBody   string
	noLocation bool // suppress the Location header on the init response
	putStatus  int
	putBody    string
}

func newUploadFixture(t *testing.T) *uploadFixture {
	t.Helper()
	f := &uploadFixture{
		initStatus: http.StatusOK,
		putStatus:  http.StatusOK,
		putBody:    `{"id":"vid42"}`,
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec := &uploadRecorded{
			Query:         r.URL.Query(),
			Header:        r.Header.Clone(),
			ContentLength: r.ContentLength,
			Body:          body,
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/upload/youtube/v3/videos":
			f.init = rec
			if !f.noLocation {
				w.Header().Set("Location", f.srv.URL+sessionPath)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.initStatus)
			_, _ = w.Write([]byte(f.initBody))
		case r.Method == http.MethodPut && r.URL.Path == sessionPath:
			f.put = rec
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.putStatus)
			_, _ = w.Write([]byte(f.putBody))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"status":"NOT_FOUND","message":"no route"}}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *uploadFixture) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		BaseURL:       f.srv.URL + "/youtube/v3",
		UploadBaseURL: f.srv.URL + "/upload/youtube/v3",
		HC:            f.srv.Client(),
		Out:           &out,
		Err:           &errBuf,
	}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// writeVideoFile drops content into a temp file and returns its path.
func writeVideoFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp video: %v", err)
	}
	return path
}

func TestVideosUpload_HappyPathJSON(t *testing.T) {
	f := newUploadFixture(t)
	f.putBody = `{"id":"vid42","snippet":{"title":"My Title"},"status":{"privacyStatus":"unlisted"}}`
	content := bytes.Repeat([]byte("abc123xyz"), 300)
	path := writeVideoFile(t, "clip.mp4", content)

	result, stdout, stderr := f.run(t, "videos", "upload",
		"--file", path, "--title", "My Title", "--description", "A demo",
		"--tags", "go, cli", "--category-id", "22", "--privacy", "unlisted", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	if f.init == nil || f.put == nil {
		t.Fatalf("init recorded = %t, put recorded = %t, want both", f.init != nil, f.put != nil)
	}

	// init: query + headers.
	if got := f.init.Query.Get("uploadType"); got != "resumable" {
		t.Errorf("init uploadType = %q, want resumable", got)
	}
	if got := f.init.Query.Get("part"); got != "snippet,status" {
		t.Errorf("init part = %q, want snippet,status", got)
	}
	if got := f.init.Header.Get("Authorization"); got != "Bearer ya29.test-token" {
		t.Errorf("init Authorization = %q, want the bearer token", got)
	}
	if got := f.init.Header.Get("X-Upload-Content-Length"); got != strconv.Itoa(len(content)) {
		t.Errorf("X-Upload-Content-Length = %q, want %d", got, len(content))
	}
	if got := f.init.Header.Get("X-Upload-Content-Type"); got != "video/mp4" {
		t.Errorf("X-Upload-Content-Type = %q, want video/mp4", got)
	}

	// init: metadata body.
	var meta struct {
		Snippet struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
			CategoryID  string   `json:"categoryId"`
		} `json:"snippet"`
		Status struct {
			PrivacyStatus string `json:"privacyStatus"`
			MadeForKids   *bool  `json:"selfDeclaredMadeForKids"`
		} `json:"status"`
	}
	if err := json.Unmarshal(f.init.Body, &meta); err != nil {
		t.Fatalf("init body not JSON: %v (%q)", err, f.init.Body)
	}
	if meta.Snippet.Title != "My Title" || meta.Snippet.Description != "A demo" || meta.Snippet.CategoryID != "22" {
		t.Errorf("init snippet = %+v, want title/description/categoryId", meta.Snippet)
	}
	if len(meta.Snippet.Tags) != 2 || meta.Snippet.Tags[0] != "go" || meta.Snippet.Tags[1] != "cli" {
		t.Errorf("init tags = %v, want [go cli]", meta.Snippet.Tags)
	}
	if meta.Status.PrivacyStatus != "unlisted" {
		t.Errorf("init privacyStatus = %q, want unlisted", meta.Status.PrivacyStatus)
	}
	if meta.Status.MadeForKids == nil || *meta.Status.MadeForKids {
		t.Errorf("init selfDeclaredMadeForKids = %v, want explicit false", meta.Status.MadeForKids)
	}

	// PUT: auth + explicit Content-Length + verbatim bytes.
	if got := f.put.Header.Get("Authorization"); got != "Bearer ya29.test-token" {
		t.Errorf("PUT Authorization = %q, want the bearer token", got)
	}
	if f.put.ContentLength != int64(len(content)) {
		t.Errorf("PUT ContentLength = %d, want %d (must be explicit, not chunked)", f.put.ContentLength, len(content))
	}
	if got := f.put.Header.Get("Content-Type"); got != "video/mp4" {
		t.Errorf("PUT Content-Type = %q, want video/mp4", got)
	}
	if !bytes.Equal(f.put.Body, content) {
		t.Errorf("PUT body = %d bytes, want the file verbatim (%d bytes)", len(f.put.Body), len(content))
	}

	// stdout: success resource verbatim.
	if strings.TrimSpace(stdout) != f.putBody {
		t.Errorf("stdout = %q, want the video resource verbatim %q", stdout, f.putBody)
	}
}

func TestVideosUpload_HumanSummary(t *testing.T) {
	f := newUploadFixture(t)
	f.putBody = `{"id":"vid42","snippet":{"title":"My Title"},"status":{"privacyStatus":"private"}}`
	path := writeVideoFile(t, "clip.mp4", []byte("video-bytes"))

	result, stdout, stderr := f.run(t, "videos", "upload", "--file", path, "--title", "My Title")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	for _, want := range []string{"uploaded video vid42", "My Title", "(private)", "https://youtu.be/vid42"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want it to contain %q", stdout, want)
		}
	}

	// --privacy defaults to private in the init metadata.
	var meta struct {
		Status struct {
			PrivacyStatus string `json:"privacyStatus"`
		} `json:"status"`
	}
	if err := json.Unmarshal(f.init.Body, &meta); err != nil {
		t.Fatalf("init body not JSON: %v", err)
	}
	if meta.Status.PrivacyStatus != "private" {
		t.Errorf("default privacyStatus = %q, want private", meta.Status.PrivacyStatus)
	}
}

func TestVideosUpload_UsageErrors_Exit2(t *testing.T) {
	emptyFile := writeVideoFile(t, "empty.mp4", nil)
	dir := t.TempDir()
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing title", []string{"videos", "upload", "--file", "clip.mp4"}, "--title is required"},
		{"missing file", []string{"videos", "upload", "--title", "T"}, "--file is required"},
		{"bad privacy", []string{"videos", "upload", "--file", "clip.mp4", "--title", "T", "--privacy", "secret"}, "--privacy must be"},
		{"nonexistent file", []string{"videos", "upload", "--file", "/nonexistent/clip.mp4", "--title", "T"}, "read video file"},
		{"file is a directory", []string{"videos", "upload", "--file", dir, "--title", "T"}, "is a directory"},
		{"empty file", []string{"videos", "upload", "--file", emptyFile, "--title", "T"}, "is empty"},
	}
	f := newUploadFixture(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 2 {
				t.Fatalf("exit code = %d, want 2 (usage)", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if f.init != nil || f.put != nil {
		t.Error("usage failures must not reach the API")
	}
}

func TestVideosUpload_InitMissingLocation_Exit1(t *testing.T) {
	f := newUploadFixture(t)
	f.noLocation = true
	path := writeVideoFile(t, "clip.mp4", []byte("video-bytes"))

	result, _, stderr := f.run(t, "videos", "upload", "--file", path, "--title", "T")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (api)", result.ExitCode)
	}
	if !strings.Contains(stderr, "no Location header") {
		t.Errorf("stderr = %q, want the explicit missing-Location error", stderr)
	}
	if f.put != nil {
		t.Error("a missing Location must not attempt the PUT")
	}
}

func TestVideosUpload_Put4xx_APIError(t *testing.T) {
	t.Run("401 rejects credential with scope hint", func(t *testing.T) {
		f := newUploadFixture(t)
		f.putStatus = http.StatusUnauthorized
		f.putBody = `{"error":{"code":401,"status":"UNAUTHENTICATED","message":"invalid credentials"}}`
		path := writeVideoFile(t, "clip.mp4", []byte("video-bytes"))

		result, _, stderr := f.run(t, "videos", "upload", "--file", path, "--title", "T")
		if result.ExitCode != 1 {
			t.Fatalf("exit code = %d, want 1 (api)", result.ExitCode)
		}
		if !strings.Contains(stderr, "invalid credentials") {
			t.Errorf("stderr = %q, want the provider message via apiMessage", stderr)
		}
		if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
			t.Errorf("stderr = %q, want the reconnect hint on 401", stderr)
		}
		if !result.CredentialRejected {
			t.Error("401 UNAUTHENTICATED on the upload PUT must reject the credential")
		}
	})

	t.Run("400 surfaces provider message without rejection", func(t *testing.T) {
		f := newUploadFixture(t)
		f.putStatus = http.StatusBadRequest
		f.putBody = `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"bad metadata"}}`
		path := writeVideoFile(t, "clip.mp4", []byte("video-bytes"))

		result, _, stderr := f.run(t, "videos", "upload", "--file", path, "--title", "T", "--json")
		if result.ExitCode != 1 {
			t.Fatalf("exit code = %d, want 1 (api)", result.ExitCode)
		}
		var env struct {
			Error struct {
				Message string `json:"message"`
				Kind    string `json:"kind"`
				Status  int    `json:"status"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
			t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
		}
		if env.Error.Kind != "api" || env.Error.Status != 400 || !strings.Contains(env.Error.Message, "bad metadata") {
			t.Errorf("envelope = %+v, want api kind + status 400 + provider message", env.Error)
		}
		if result.CredentialRejected {
			t.Error("400 INVALID_ARGUMENT must not reject the credential")
		}
	})
}
