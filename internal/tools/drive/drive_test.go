package drive

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// recordedRequest is one request the fake Drive server saw.
type recordedRequest struct {
	Method      string
	Path        string
	Query       string
	Auth        string
	ContentType string
	Body        []byte
}

// route is a canned response for "METHOD /path".
type route struct {
	status  int
	body    string
	headers map[string]string
}

// fixture is a fake Drive API server: routes keyed by "METHOD /path", every
// request recorded in order. Retry backoff sleeps are recorded, not slept, so
// tests stay fast and deterministic.
type fixture struct {
	srv      *httptest.Server
	requests []recordedRequest
	sleeps   []time.Duration
}

func newFixture(t *testing.T, routes map[string]route) *fixture {
	t.Helper()
	f := &fixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.requests = append(f.requests, recordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body.Bytes(),
		})
		rt, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"no route"}}`))
			return
		}
		for k, v := range rt.headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// last returns the most recent request matching method+path.
func (f *fixture) last(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method == method && f.requests[i].Path == path {
			return f.requests[i]
		}
	}
	t.Fatalf("no recorded request %s %s", method, path)
	return recordedRequest{}
}

func (f *fixture) newService() *Service {
	return &Service{
		BaseURL:       f.srv.URL + "/drive/v3",
		UploadBaseURL: f.srv.URL + "/upload/drive/v3",
		HC:            f.srv.Client(),
		sleep:         func(d time.Duration) { f.sleeps = append(f.sleeps, d) },
	}
}

func (f *fixture) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := f.newService()
	svc.Out = &out
	svc.Err = &errBuf
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func (f *fixture) runOK(t *testing.T, args ...string) string {
	t.Helper()
	result, stdout, stderr := f.run(t, args...)
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	return stdout
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"about"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "DRIVE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"files", "explode"}, "explode"},
		{"get without id", []string{"files", "get"}, "accepts 1 arg"},
		{"export bad format", []string{"files", "export", "f1", "--format", "rtf"}, "unsupported --format"},
		{"export missing format", []string{"files", "export", "f1"}, `required flag(s) "format"`},
		{"update nothing", []string{"files", "update", "f1"}, "nothing to update"},
		{"share bad role", []string{"files", "share", "f1", "--with", "a@b.c", "--role", "owner"}, "reader, commenter, or writer"},
		{"share anyone and with", []string{"files", "share", "f1", "--anyone", "--with", "a@b.c", "--role", "reader"}, "mutually exclusive"},
		{"share no target", []string{"files", "share", "f1", "--role", "reader"}, "needs --with"},
		{"trash no ids", []string{"files", "trash", " ", ""}, "no valid file ids"},
	}
	f := newFixture(t, map[string]route{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("argv failures must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestAbout_HumanAndJSON(t *testing.T) {
	body := `{"user":{"displayName":"Ada","emailAddress":"ada@example.com"},"storageQuota":{"limit":"1000","usage":"250","usageInDrive":"200"}}`
	f := newFixture(t, map[string]route{
		"GET /drive/v3/about": {status: http.StatusOK, body: body},
	})
	stdout := f.runOK(t, "about")
	if !strings.Contains(stdout, "ada@example.com") || !strings.Contains(stdout, "250 / 1000") {
		t.Errorf("human output = %q, want account + usage", stdout)
	}
	got := f.last(t, "GET", "/drive/v3/about")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}
	if !strings.Contains(got.Query, "fields=") {
		t.Errorf("query = %q, want a fields mask (about.get requires it)", got.Query)
	}
	stdout = f.runOK(t, "about", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestFilesList_QueryAndParentComposition(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files": {status: http.StatusOK, body: `{"files":[{"id":"f1","name":"report.pdf","mimeType":"application/pdf"}],"nextPageToken":"npt-2"}`},
	})
	stdout := f.runOK(t, "files", "list", "--query", "name contains 'report'", "--parent", "folder9", "--max", "5")
	got := f.last(t, "GET", "/drive/v3/files")
	// q must AND the --query and the parent clause; supportsAllDrives透传.
	for _, want := range []string{"q=", "in+parents", "name+contains", "supportsAllDrives=true", "includeItemsFromAllDrives=true", "pageSize=5"} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want it to contain %q", got.Query, want)
		}
	}
	if !strings.Contains(stdout, "f1") || !strings.Contains(stdout, "next page token: npt-2") {
		t.Errorf("human output = %q, want ids + next page token", stdout)
	}
}

func TestFilesList_DefaultsToLiveFiles(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files": {status: http.StatusOK, body: `{"files":[]}`},
	})
	f.runOK(t, "files", "list")
	if got := decodeQ(t, f.last(t, "GET", "/drive/v3/files").Query); got != "trashed = false" {
		t.Errorf("q = %q, want the default live-files filter", got)
	}
}

func TestFilesList_RespectsExplicitTrashedQuery(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files": {status: http.StatusOK, body: `{"files":[{"id":"t1","name":"old.pdf","mimeType":"application/pdf","trashed":true}]}`},
	})
	stdout := f.runOK(t, "files", "list", "--query", "trashed = true")
	// The caller's own trashed clause must be passed through untouched — no
	// second injected trashed=false.
	if got := decodeQ(t, f.last(t, "GET", "/drive/v3/files").Query); got != "trashed = true" {
		t.Errorf("q = %q, want the caller's trashed clause verbatim", got)
	}
	if !strings.Contains(stdout, "(trashed)") {
		t.Errorf("human output = %q, want a trashed marker on trashed files", stdout)
	}
}

func TestFilesList_MaxOutOfRange(t *testing.T) {
	f := newFixture(t, map[string]route{})
	for _, m := range []string{"0", "-1", "1001"} {
		result, _, stderr := f.run(t, "files", "list", "--max", m)
		if result.ExitCode != 1 {
			t.Fatalf("--max %s exit = %d, want 1", m, result.ExitCode)
		}
		if !strings.Contains(stderr, "--max must be between 1 and 1000") {
			t.Errorf("--max %s stderr = %q, want the local range error", m, stderr)
		}
	}
	if len(f.requests) != 0 {
		t.Errorf("out-of-range --max must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestFilesList_EmptyExplainsVisibility(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files": {status: http.StatusOK, body: `{"files":[]}`},
	})
	stdout := f.runOK(t, "files", "list")
	if !strings.Contains(stdout, "drive.file only sees files Helio created") {
		t.Errorf("empty output = %q, want the visibility explanation", stdout)
	}
}

func TestFilesGet_MetadataAndVisibility404(t *testing.T) {
	body := `{"id":"f1","name":"deliverable.pdf","mimeType":"application/pdf","size":"2048","parents":["p1"],"owners":[{"displayName":"Ada","emailAddress":"ada@x.com"}],"webViewLink":"https://drive.google.com/file/d/f1/view","permissions":[{"role":"reader","type":"anyone"}]}`
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files/f1": {status: http.StatusOK, body: body},
	})
	stdout := f.runOK(t, "files", "get", "f1")
	for _, want := range []string{"deliverable.pdf", "drive.google.com/file/d/f1/view", "reader", "anyone"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}

	// A 404 on a file outside the drive.file domain must carry the boundary hint.
	f2 := newFixture(t, map[string]route{
		"GET /drive/v3/files/missing": {status: http.StatusNotFound, body: `{"error":{"status":"NOT_FOUND","message":"File not found"}}`},
	})
	result, _, stderr := f2.run(t, "files", "get", "missing")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "outside this tool's authorization") {
		t.Errorf("stderr = %q, want the drive.file visibility hint", stderr)
	}
	if result.CredentialRejected {
		t.Error("404 must not reject the credential")
	}
}

func TestFilesMkdir_FolderMime(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /drive/v3/files": {status: http.StatusOK, body: `{"id":"dir1","name":"Reports","mimeType":"application/vnd.google-apps.folder"}`},
	})
	stdout := f.runOK(t, "files", "mkdir", "Reports", "--parent", "root9")
	got := f.last(t, "POST", "/drive/v3/files")
	if !strings.Contains(string(got.Body), `"mimeType":"application/vnd.google-apps.folder"`) {
		t.Errorf("body = %s, want the folder mimeType", got.Body)
	}
	if !strings.Contains(string(got.Body), `"parents":["root9"]`) {
		t.Errorf("body = %s, want the parent", got.Body)
	}
	if !strings.Contains(stdout, "created folder Reports (dir1)") {
		t.Errorf("human output = %q", stdout)
	}
}

func TestFilesUpdate_MoveUsesAddRemoveParents(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /drive/v3/files/f1": {status: http.StatusOK, body: `{"id":"f1","name":"new.txt"}`},
	})
	f.runOK(t, "files", "update", "f1", "--name", "new.txt", "--parent", "dest", "--remove-parent", "src")
	got := f.last(t, "PATCH", "/drive/v3/files/f1")
	for _, want := range []string{"addParents=dest", "removeParents=src"} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want %q", got.Query, want)
		}
	}
	if !strings.Contains(string(got.Body), `"name":"new.txt"`) {
		t.Errorf("body = %s, want the rename", got.Body)
	}
}

func TestFilesTrash_SetsTrashedFlag(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /drive/v3/files/f1": {status: http.StatusOK, body: `{"id":"f1","trashed":true}`},
		"PATCH /drive/v3/files/f2": {status: http.StatusOK, body: `{"id":"f2","trashed":true}`},
	})
	stdout := f.runOK(t, "files", "trash", "f1", "f2", "--json")
	if len(f.requests) != 2 {
		t.Fatalf("saw %d requests, want one patch per id", len(f.requests))
	}
	got := f.last(t, "PATCH", "/drive/v3/files/f1")
	if !strings.Contains(string(got.Body), `"trashed":true`) {
		t.Errorf("body = %s, want trashed=true", got.Body)
	}
	if !strings.Contains(stdout, `"status":"trashed"`) {
		t.Errorf("--json output = %q, want the trashed status", stdout)
	}
}

func TestFilesUntrash_SetsTrashedFalse(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /drive/v3/files/f1": {status: http.StatusOK, body: `{"id":"f1","trashed":false}`},
	})
	stdout := f.runOK(t, "files", "untrash", "f1")
	got := f.last(t, "PATCH", "/drive/v3/files/f1")
	if !strings.Contains(string(got.Body), `"trashed":false`) {
		t.Errorf("body = %s, want trashed=false", got.Body)
	}
	if !strings.Contains(stdout, "untrashed 1 file(s)") {
		t.Errorf("human output = %q", stdout)
	}
}

func TestFilesDownload_AltMediaAndSave(t *testing.T) {
	dir := t.TempDir()
	// files.get is called twice: once for the name (JSON), once with alt=media
	// (blob). Serve JSON metadata unless alt=media is requested.
	f := newFixtureFunc(t, func(_ *fixture, w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/drive/v3/files/f1" {
			if r.URL.Query().Get("alt") == "media" {
				_, _ = w.Write([]byte("raw-bytes-here"))
				return true
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"f1","name":"data.bin","mimeType":"application/octet-stream"}`))
			return true
		}
		return false
	})
	stdout := f.runOK(t, "files", "download", "f1", "--save", dir, "--json")
	if !strings.Contains(stdout, `"size":14`) || !strings.Contains(stdout, "data.bin") {
		t.Errorf("--json output = %q, want the saved-file record", stdout)
	}
	got := f.last(t, "GET", "/drive/v3/files/f1")
	if got.Query == "" || !strings.Contains(got.Query, "alt=media") {
		t.Errorf("query = %q, want alt=media on the content fetch", got.Query)
	}
}

func TestFilesExport_FormatMimeMapping(t *testing.T) {
	dir := t.TempDir()
	f := newFixtureFunc(t, func(_ *fixture, w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/drive/v3/files/doc1" && r.URL.Query().Get("mimeType") == "":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"doc1","name":"Quarterly","mimeType":"application/vnd.google-apps.document"}`))
			return true
		case r.URL.Path == "/drive/v3/files/doc1/export":
			_, _ = w.Write([]byte("%PDF-1.7 fake"))
			return true
		}
		return false
	})
	stdout := f.runOK(t, "files", "export", "doc1", "--format", "pdf", "--save", dir, "--json")
	got := f.last(t, "GET", "/drive/v3/files/doc1/export")
	if !strings.Contains(got.Query, "mimeType=application%2Fpdf") {
		t.Errorf("query = %q, want the pdf export mimeType", got.Query)
	}
	// files.export accepts only fileId and mimeType; supportsAllDrives is
	// undefined for this method and must not be sent.
	if strings.Contains(got.Query, "supportsAllDrives") {
		t.Errorf("query = %q, must not carry supportsAllDrives on files.export", got.Query)
	}
	if !strings.Contains(stdout, "Quarterly.pdf") {
		t.Errorf("--json output = %q, want the exported filename with .pdf", stdout)
	}
}

func TestFilesUpload_MultipartBody(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "notes.txt", "hello drive")
	f := newFixture(t, map[string]route{
		"POST /upload/drive/v3/files": {status: http.StatusOK, body: `{"id":"up1","name":"notes.txt","webViewLink":"https://drive.google.com/file/d/up1/view"}`},
	})
	stdout := f.runOK(t, "files", "upload", path, "--parent", "folder1")
	got := f.last(t, "POST", "/upload/drive/v3/files")
	if !strings.Contains(got.Query, "uploadType=multipart") {
		t.Errorf("query = %q, want uploadType=multipart", got.Query)
	}
	if !strings.HasPrefix(got.ContentType, "multipart/related; boundary=") {
		t.Errorf("content-type = %q, want multipart/related", got.ContentType)
	}
	bodyStr := string(got.Body)
	if !strings.Contains(bodyStr, `"name":"notes.txt"`) || !strings.Contains(bodyStr, `"parents":["folder1"]`) {
		t.Errorf("body = %s, want metadata part with name + parents", bodyStr)
	}
	if !strings.Contains(bodyStr, "hello drive") {
		t.Errorf("body = %s, want the media part content", bodyStr)
	}
	if !strings.Contains(stdout, "drive.google.com/file/d/up1/view") {
		t.Errorf("human output = %q, want the webViewLink", stdout)
	}
}

func TestFilesUpload_ConvertSetsWorkspaceMime(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "sheet.csv", "a,b\n1,2\n")
	f := newFixture(t, map[string]route{
		"POST /upload/drive/v3/files": {status: http.StatusOK, body: `{"id":"s1","name":"sheet","webViewLink":"https://drive.google.com/x"}`},
	})
	f.runOK(t, "files", "upload", path, "--convert")
	got := f.last(t, "POST", "/upload/drive/v3/files")
	if !strings.Contains(string(got.Body), `"mimeType":"application/vnd.google-apps.spreadsheet"`) {
		t.Errorf("body = %s, want the spreadsheet conversion target", got.Body)
	}
}

func TestFilesUpload_LargeUsesResumable(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "big.bin", strings.Repeat("x", 32))
	sessionPath := "/upload/drive/v3/session/xyz"
	f := newFixtureFunc(t, func(f *fixture, w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
			// initiation: hand back a session URI on the same server.
			w.Header().Set("Location", f.srv.URL+sessionPath)
			w.WriteHeader(http.StatusOK)
			return true
		case r.Method == http.MethodPut && r.URL.Path == sessionPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"big1","name":"big.bin","webViewLink":"https://drive.google.com/big"}`))
			return true
		}
		return false
	})
	// Force the resumable path with a tiny threshold.
	svc := f.newService()
	svc.resumableThreshold = 8
	var out bytes.Buffer
	svc.Out = &out
	result, err := svc.Execute(context.Background(), []string{"files", "upload", path, "--json"}, map[string]string{EnvAccessToken: "tok"})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("resumable upload failed: err=%v exit=%d", err, result.ExitCode)
	}
	init := f.last(t, "POST", "/upload/drive/v3/files")
	if !strings.Contains(init.Query, "uploadType=resumable") {
		t.Errorf("init query = %q, want uploadType=resumable", init.Query)
	}
	if got := init.Body; !strings.Contains(string(got), `"name":"big.bin"`) {
		t.Errorf("init body = %s, want the metadata (no media)", got)
	}
	put := f.last(t, "PUT", sessionPath)
	if string(put.Body) != strings.Repeat("x", 32) {
		t.Errorf("PUT body length = %d, want the full media", len(put.Body))
	}
	if !strings.Contains(out.String(), "big1") {
		t.Errorf("output = %q, want the uploaded id", out.String())
	}
}

func TestPermissionsList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files/f1/permissions": {status: http.StatusOK, body: `{"permissions":[{"id":"p1","type":"user","role":"writer","emailAddress":"bob@x.com"},{"id":"pA","type":"anyone","role":"reader"}]}`},
	})
	stdout := f.runOK(t, "permissions", "list", "f1")
	for _, want := range []string{"bob@x.com", "writer", "anyone", "reader"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output = %q, want %q", stdout, want)
		}
	}
}

func TestFilesShare_WithUsers(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /drive/v3/files/f1/permissions": {status: http.StatusOK, body: `{"id":"p1","type":"user","role":"writer","emailAddress":"bob@x.com"}`},
	})
	stdout := f.runOK(t, "files", "share", "f1", "--with", "bob@x.com", "--role", "writer", "--message", "here you go")
	got := f.last(t, "POST", "/drive/v3/files/f1/permissions")
	if !strings.Contains(string(got.Body), `"type":"user"`) || !strings.Contains(string(got.Body), `"emailAddress":"bob@x.com"`) || !strings.Contains(string(got.Body), `"role":"writer"`) {
		t.Errorf("body = %s, want a user writer grant", got.Body)
	}
	if !strings.Contains(got.Query, "emailMessage=here") {
		t.Errorf("query = %q, want the email message", got.Query)
	}
	if !strings.Contains(stdout, "shared f1 as writer with bob@x.com") {
		t.Errorf("output = %q", stdout)
	}
}

func TestFilesShare_AnyoneLink(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /drive/v3/files/f1/permissions": {status: http.StatusOK, body: `{"id":"pA","type":"anyone","role":"reader"}`},
	})
	f.runOK(t, "files", "share", "f1", "--anyone", "--role", "reader")
	got := f.last(t, "POST", "/drive/v3/files/f1/permissions")
	if !strings.Contains(string(got.Body), `"type":"anyone"`) {
		t.Errorf("body = %s, want an anyone grant", got.Body)
	}
}

func TestFilesShare_MultipleWithSplitsAndNoNotify(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /drive/v3/files/f1/permissions": {status: http.StatusOK, body: `{"id":"p1","type":"user","role":"reader","emailAddress":"a@x.com"}`},
	})
	f.runOK(t, "files", "share", "f1", "--with", "a@x.com,b@x.com", "--with", "c@x.com", "--role", "reader", "--no-notify")
	if len(f.requests) != 3 {
		t.Fatalf("saw %d requests, want one permission create per recipient", len(f.requests))
	}
	got := f.last(t, "POST", "/drive/v3/files/f1/permissions")
	if !strings.Contains(got.Query, "sendNotificationEmail=false") {
		t.Errorf("query = %q, want sendNotificationEmail=false", got.Query)
	}
}

func TestPermissionsUpdate_Escalation(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /drive/v3/files/f1/permissions/p1": {status: http.StatusOK, body: `{"id":"p1","role":"writer"}`},
	})
	stdout := f.runOK(t, "permissions", "update", "f1", "p1", "--role", "writer")
	got := f.last(t, "PATCH", "/drive/v3/files/f1/permissions/p1")
	if !strings.Contains(string(got.Body), `"role":"writer"`) {
		t.Errorf("body = %s, want the new role", got.Body)
	}
	if !strings.Contains(stdout, "updated permission p1 to writer") {
		t.Errorf("output = %q", stdout)
	}
}

func TestPermissionsDelete(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /drive/v3/files/f1/permissions/p1": {status: http.StatusNoContent, body: ""},
	})
	stdout := f.runOK(t, "permissions", "delete", "f1", "p1", "--json")
	if !strings.Contains(stdout, `"status":"deleted"`) {
		t.Errorf("--json output = %q, want the deleted status", stdout)
	}
}

func TestScopeHintOn403(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files": {status: http.StatusForbidden, body: `{"error":{"status":"PERMISSION_DENIED","message":"insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "files", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") || !strings.Contains(stderr, "possibly missing scope") {
		t.Errorf("stderr = %q, want the provider message + reconnect hint", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 PERMISSION_DENIED must not reject the credential")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		providerStatus string
		wantRejected   bool
	}{
		{"http unauthorized", http.StatusUnauthorized, "UNKNOWN", true},
		{"explicit unauthenticated", http.StatusBadRequest, "UNAUTHENTICATED", true},
		{"permission denied", http.StatusForbidden, "PERMISSION_DENIED", false},
		{"not found", http.StatusNotFound, "NOT_FOUND", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /drive/v3/about": {status: tc.status, body: `{"error":{"status":"` + tc.providerStatus + `","message":"m"}}`},
			})
			result, _, _ := f.run(t, "about")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}
