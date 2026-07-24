package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testVideoURN is the raw (unencoded) URN the fake init endpoint hands out.
const testVideoURN = "urn:li:video:987"

// wantPollPath is the literal percent-encoded path the poll GET must hit.
// Deliberately a literal — NOT built with the client's encoding helper — so
// both sides can't share the same bug (design §3.4).
const wantPollPath = "/rest/videos/urn%3Ali%3Avideo%3A987"

// versionedCapture records one request to a versioned /rest/videos action.
type versionedCapture struct {
	Method   string
	Action   string
	Auth     string
	Version  string
	Protocol string
	Body     []byte
}

// partCapture records one chunk PUT as the fake upload host saw it.
type partCapture struct {
	Path          string
	ContentLength int64
	ContentType   string
	Auth          string
	Version       string
	Body          []byte
}

// pollCapture records the poll GETs against /rest/videos/{urn}.
type pollCapture struct {
	EscapedPath string
	RequestURI  string
	Auth        string
	Version     string
	Protocol    string
	Count       int
}

// videoServer is a fake LinkedIn video API: initializeUpload / chunk PUTs /
// finalizeUpload / status GET, with per-endpoint captures.
type videoServer struct {
	srv      *httptest.Server
	init     versionedCapture
	parts    []partCapture
	finalize versionedCapture
	poll     pollCapture
}

// newVideoServer builds the fake. ranges are the server-chosen upload
// instructions ([firstByte, lastByte] pairs, uploadUrl pointing back at the
// fake). pollBodies are returned in order to successive poll GETs (the last
// one repeats).
func newVideoServer(t *testing.T, ranges [][2]int64, pollBodies []string) *videoServer {
	t.Helper()
	vs := &videoServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/videos", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c := versionedCapture{
			Method:   r.Method,
			Action:   r.URL.Query().Get("action"),
			Auth:     r.Header.Get("Authorization"),
			Version:  r.Header.Get("LinkedIn-Version"),
			Protocol: r.Header.Get("X-Restli-Protocol-Version"),
			Body:     body,
		}
		w.Header().Set("Content-Type", "application/json")
		switch c.Action {
		case "initializeUpload":
			vs.init = c
			instructions := make([]string, 0, len(ranges))
			for i, rg := range ranges {
				instructions = append(instructions, fmt.Sprintf(
					`{"uploadUrl":%q,"firstByte":%d,"lastByte":%d}`,
					fmt.Sprintf("%s/videoUpload/%d", vs.srv.URL, i), rg[0], rg[1]))
			}
			fmt.Fprintf(w, `{"value":{"video":%q,"uploadInstructions":[%s]}}`,
				testVideoURN, strings.Join(instructions, ","))
		case "finalizeUpload":
			vs.finalize = c
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected action %q on /rest/videos", c.Action)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/videoUpload/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vs.parts = append(vs.parts, partCapture{
			Path:          r.URL.Path,
			ContentLength: r.ContentLength,
			ContentType:   r.Header.Get("Content-Type"),
			Auth:          r.Header.Get("Authorization"),
			Version:       r.Header.Get("LinkedIn-Version"),
			Body:          body,
		})
		w.Header().Set("ETag", fmt.Sprintf("etag-%d", len(vs.parts)-1))
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/rest/videos/", func(w http.ResponseWriter, r *http.Request) {
		// r.URL.EscapedPath(), not r.URL.Path: Go decodes Path back to bare
		// colons, which would hide a missing client-side encoding.
		vs.poll = pollCapture{
			EscapedPath: r.URL.EscapedPath(),
			RequestURI:  r.RequestURI,
			Auth:        r.Header.Get("Authorization"),
			Version:     r.Header.Get("LinkedIn-Version"),
			Protocol:    r.Header.Get("X-Restli-Protocol-Version"),
			Count:       vs.poll.Count + 1,
		}
		if len(pollBodies) == 0 {
			t.Errorf("unexpected poll GET %s", r.RequestURI)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		i := vs.poll.Count - 1
		if i >= len(pollBodies) {
			i = len(pollBodies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, pollBodies[i])
	})
	vs.srv = httptest.NewServer(mux)
	t.Cleanup(vs.srv.Close)
	return vs
}

func writeTempVideo(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp video: %v", err)
	}
	return path
}

func TestVideoUpload_Happy(t *testing.T) {
	data := []byte("0123456789")
	available := `{"id":"urn:li:video:987","status":"AVAILABLE"}`
	vs := newVideoServer(t, [][2]int64{{0, 3}, {4, 9}}, []string{available})
	file := writeTempVideo(t, "clip.mp4", data)

	code, stdout, _ := run(t, vs.srv, fullEnv(), "video", "upload", "--file", file)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	// init
	if vs.init.Method != http.MethodPost {
		t.Errorf("init method = %q, want POST", vs.init.Method)
	}
	if vs.init.Auth != "Bearer li-token" {
		t.Errorf("init Authorization = %q, want Bearer li-token", vs.init.Auth)
	}
	if vs.init.Version != "202607" || vs.init.Protocol != "2.0.0" {
		t.Errorf("init versioned headers = (%q, %q), want (202607, 2.0.0)", vs.init.Version, vs.init.Protocol)
	}
	var initReq struct {
		InitializeUploadRequest struct {
			Owner         string `json:"owner"`
			FileSizeBytes int64  `json:"fileSizeBytes"`
		} `json:"initializeUploadRequest"`
	}
	if err := json.Unmarshal(vs.init.Body, &initReq); err != nil {
		t.Fatalf("init body not JSON: %v", err)
	}
	if initReq.InitializeUploadRequest.Owner != "urn:li:person:abc123" {
		t.Errorf("init owner = %q, want the injected person URN", initReq.InitializeUploadRequest.Owner)
	}
	if initReq.InitializeUploadRequest.FileSizeBytes != int64(len(data)) {
		t.Errorf("init fileSizeBytes = %d, want %d", initReq.InitializeUploadRequest.FileSizeBytes, len(data))
	}

	// chunk PUTs: order, byte ranges, explicit Content-Length, bare headers.
	wantParts := []struct {
		Path string
		Body string
	}{
		{Path: "/videoUpload/0", Body: "0123"},
		{Path: "/videoUpload/1", Body: "456789"},
	}
	if len(vs.parts) != len(wantParts) {
		t.Fatalf("got %d chunk PUTs, want %d", len(vs.parts), len(wantParts))
	}
	for i, want := range wantParts {
		got := vs.parts[i]
		if got.Path != want.Path {
			t.Errorf("part %d path = %q, want %q", i, got.Path, want.Path)
		}
		if string(got.Body) != want.Body {
			t.Errorf("part %d body = %q, want %q", i, got.Body, want.Body)
		}
		// Pins the explicit req.ContentLength: an *io.SectionReader body
		// otherwise goes out chunked (ContentLength -1 server-side).
		if got.ContentLength <= 0 || got.ContentLength != int64(len(want.Body)) {
			t.Errorf("part %d ContentLength = %d, want %d", i, got.ContentLength, len(want.Body))
		}
		if got.ContentType != "application/octet-stream" {
			t.Errorf("part %d Content-Type = %q, want application/octet-stream", i, got.ContentType)
		}
		if got.Auth != "" {
			t.Errorf("part %d Authorization = %q, want unset on pre-signed upload URL", i, got.Auth)
		}
		if got.Version != "" {
			t.Errorf("part %d LinkedIn-Version = %q, want unset on chunk PUT", i, got.Version)
		}
	}

	// finalize: uploadedPartIds are the ETag headers in instruction order.
	if vs.finalize.Method != http.MethodPost {
		t.Errorf("finalize method = %q, want POST", vs.finalize.Method)
	}
	if vs.finalize.Version != "202607" || vs.finalize.Protocol != "2.0.0" {
		t.Errorf("finalize versioned headers = (%q, %q), want (202607, 2.0.0)", vs.finalize.Version, vs.finalize.Protocol)
	}
	var finReq struct {
		FinalizeUploadRequest struct {
			Video           string   `json:"video"`
			UploadToken     string   `json:"uploadToken"`
			UploadedPartIds []string `json:"uploadedPartIds"`
		} `json:"finalizeUploadRequest"`
	}
	if err := json.Unmarshal(vs.finalize.Body, &finReq); err != nil {
		t.Fatalf("finalize body not JSON: %v", err)
	}
	if finReq.FinalizeUploadRequest.Video != testVideoURN {
		t.Errorf("finalize video = %q, want %q", finReq.FinalizeUploadRequest.Video, testVideoURN)
	}
	if finReq.FinalizeUploadRequest.UploadToken != "" {
		t.Errorf("finalize uploadToken = %q, want empty", finReq.FinalizeUploadRequest.UploadToken)
	}
	wantIds := []string{"etag-0", "etag-1"}
	if len(finReq.FinalizeUploadRequest.UploadedPartIds) != len(wantIds) {
		t.Fatalf("uploadedPartIds = %v, want %v", finReq.FinalizeUploadRequest.UploadedPartIds, wantIds)
	}
	for i, want := range wantIds {
		if finReq.FinalizeUploadRequest.UploadedPartIds[i] != want {
			t.Errorf("uploadedPartIds[%d] = %q, want %q (ETag header, instruction order)",
				i, finReq.FinalizeUploadRequest.UploadedPartIds[i], want)
		}
	}

	// poll: literal percent-encoded path, versioned headers, single GET.
	if vs.poll.EscapedPath != wantPollPath {
		t.Errorf("poll EscapedPath = %q, want %q", vs.poll.EscapedPath, wantPollPath)
	}
	if vs.poll.RequestURI != wantPollPath {
		t.Errorf("poll RequestURI = %q, want %q", vs.poll.RequestURI, wantPollPath)
	}
	if vs.poll.Auth != "Bearer li-token" {
		t.Errorf("poll Authorization = %q, want Bearer li-token", vs.poll.Auth)
	}
	if vs.poll.Version != "202607" || vs.poll.Protocol != "2.0.0" {
		t.Errorf("poll versioned headers = (%q, %q), want (202607, 2.0.0)", vs.poll.Version, vs.poll.Protocol)
	}
	if vs.poll.Count != 1 {
		t.Errorf("poll count = %d, want 1 (first GET is AVAILABLE, zero sleep)", vs.poll.Count)
	}

	if stdout != available+"\n" {
		t.Errorf("stdout = %q, want the final GET body verbatim", stdout)
	}
}

func TestVideoUpload_ProcessingFailed(t *testing.T) {
	vs := newVideoServer(t, [][2]int64{{0, 9}},
		[]string{`{"id":"urn:li:video:987","status":"PROCESSING_FAILED"}`})
	file := writeTempVideo(t, "clip.mp4", []byte("0123456789"))

	code, _, stderr := run(t, vs.srv, fullEnv(), "video", "upload", "--file", file)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "linkedin: video processing failed (status PROCESSING_FAILED)") {
		t.Errorf("stderr = %q, want the processing-failed message", stderr)
	}
	if vs.poll.Count != 1 {
		t.Errorf("poll count = %d, want 1 (first GET is terminal, zero sleep)", vs.poll.Count)
	}
}

// TestVideoUpload_PollTimeout drives waitVideoAvailable directly with a zero
// timeout: poll-then-sleep means the first (non-terminal) GET trips the
// deadline check before any sleep, so the test does not wait.
func TestVideoUpload_PollTimeout(t *testing.T) {
	vs := newVideoServer(t, nil, []string{`{"id":"urn:li:video:987","status":"PROCESSING"}`})
	var out, errBuf bytes.Buffer
	svc := &Service{APIBase: vs.srv.URL, HC: vs.srv.Client(), Out: &out, Err: &errBuf}

	_, err := svc.waitVideoAvailable(context.Background(), "li-token", testVideoURN, 0)
	if err == nil {
		t.Fatal("waitVideoAvailable returned nil error, want timeout")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not AVAILABLE after") {
		t.Errorf("error = %q, want a not-AVAILABLE timeout message", msg)
	}
	if !strings.Contains(msg, "(last status PROCESSING)") {
		t.Errorf("error = %q, want the last polled status", msg)
	}
	// The rescue hint must carry the raw URN — escaping is video get's
	// internal concern, the AI passes the URN back verbatim.
	if !strings.Contains(msg, "check later with: linkedin video get urn:li:video:987") {
		t.Errorf("error = %q, want the video get rescue hint with the raw URN", msg)
	}
	if strings.Contains(msg, "%3A") {
		t.Errorf("error = %q, must not leak the percent-encoded URN", msg)
	}
	if vs.poll.Count != 1 {
		t.Errorf("poll count = %d, want 1 (zero timeout, zero sleep)", vs.poll.Count)
	}
}

func TestVideoGet_Happy(t *testing.T) {
	available := `{"id":"urn:li:video:987","status":"AVAILABLE"}`
	vs := newVideoServer(t, nil, []string{available})

	code, stdout, _ := run(t, vs.srv, fullEnv(), "video", "get", testVideoURN)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if vs.poll.EscapedPath != wantPollPath {
		t.Errorf("EscapedPath = %q, want %q", vs.poll.EscapedPath, wantPollPath)
	}
	if vs.poll.Version != "202607" || vs.poll.Protocol != "2.0.0" {
		t.Errorf("versioned headers = (%q, %q), want (202607, 2.0.0)", vs.poll.Version, vs.poll.Protocol)
	}
	if stdout != available+"\n" {
		t.Errorf("stdout = %q, want the GET body verbatim", stdout)
	}
}

func TestVideoUpload_MissingPersonURN(t *testing.T) {
	vs := newVideoServer(t, nil, nil)
	file := writeTempVideo(t, "clip.mp4", []byte("0123456789"))

	env := map[string]string{EnvAccessToken: "li-token"} // person_urn absent
	code, _, stderr := run(t, vs.srv, env, "video", "upload", "--file", file)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 when person_urn is missing", code)
	}
	if !strings.Contains(stderr, "person_urn missing — reconnect LinkedIn to capture it") {
		t.Errorf("stderr = %q, want the reconnect hint", stderr)
	}
	if vs.init.Method != "" {
		t.Errorf("no request must be sent without a person URN, got init %s", vs.init.Method)
	}
}

func TestVideoUpload_NonMP4(t *testing.T) {
	vs := newVideoServer(t, nil, nil)
	file := writeTempVideo(t, "clip.mov", []byte("0123456789"))

	code, _, stderr := run(t, vs.srv, fullEnv(), "video", "upload", "--file", file)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for a non-MP4 file", code)
	}
	if !strings.Contains(stderr, "linkedin: only MP4 video is supported (got .mov)") {
		t.Errorf("stderr = %q, want the MP4-only message", stderr)
	}
	if vs.init.Method != "" {
		t.Errorf("no request must be sent for a non-MP4 file, got init %s", vs.init.Method)
	}
}

// TestPostCreate_WithVideoURN pins the content.media payload and the
// synthesized {"id":…} stdout when LinkedIn answers 201 with an empty body
// and the post URN only in the x-restli-id header.
func TestPostCreate_WithVideoURN(t *testing.T) {
	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = capturedRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			Auth:     r.Header.Get("Authorization"),
			Version:  r.Header.Get("LinkedIn-Version"),
			Protocol: r.Header.Get("X-Restli-Protocol-Version"),
			Body:     body,
		}
		w.Header().Set("x-restli-id", "urn:li:share:42")
		w.WriteHeader(http.StatusCreated) // empty body
	}))
	defer srv.Close()

	code, stdout, _ := run(t, srv, fullEnv(),
		"post", "create", "--text", "Watch this", "--video-urn", testVideoURN, "--video-title", "My clip")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/rest/posts" {
		t.Errorf("request = %s %s, want POST /rest/posts", got.Method, got.Path)
	}
	var payload struct {
		Commentary string `json:"commentary"`
		Content    struct {
			Media struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"media"`
		} `json:"content"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload.Commentary != "Watch this" {
		t.Errorf("commentary = %q, want the post text", payload.Commentary)
	}
	if payload.Content.Media.ID != testVideoURN {
		t.Errorf("content.media.id = %q, want %q", payload.Content.Media.ID, testVideoURN)
	}
	if payload.Content.Media.Title != "My clip" {
		t.Errorf("content.media.title = %q, want My clip", payload.Content.Media.Title)
	}
	if stdout != `{"id":"urn:li:share:42"}`+"\n" {
		t.Errorf("stdout = %q, want the synthesized id JSON", stdout)
	}
}

func TestPostCreate_VideoTitleWithoutURN(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(), "post", "create", "--text", "x", "--video-title", "t")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--video-title requires --video-urn") {
		t.Errorf("stderr = %q, want the flag-pairing usage error", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}
