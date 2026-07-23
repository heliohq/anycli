package tiktok

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreatorInfoRequest(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"privacy_level_options":["SELF_ONLY"],"max_video_post_duration_sec":300}`))
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "creator", "info")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/post/publish/creator_info/query/" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	if !strings.HasPrefix(got.ContentType, "application/json") {
		t.Fatalf("content-type = %q, want application/json", got.ContentType)
	}
	if !strings.Contains(stdout, "privacy_level_options") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPostVideoDirectFromURL(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"publish_id":"v_pub_1"}`))
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(),
		"post", "video", "--url", "https://cdn.example.com/v.mp4", "--title", "hi", "--privacy", "SELF_ONLY")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v2/post/publish/video/init/" {
		t.Fatalf("path = %q, want direct post init", got.Path)
	}
	var body struct {
		PostInfo struct {
			PrivacyLevel       string `json:"privacy_level"`
			Title              string `json:"title"`
			BrandContentToggle bool   `json:"brand_content_toggle"`
		} `json:"post_info"`
		SourceInfo struct {
			Source   string `json:"source"`
			VideoURL string `json:"video_url"`
		} `json:"source_info"`
	}
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, got.Body)
	}
	if body.SourceInfo.Source != "PULL_FROM_URL" || body.SourceInfo.VideoURL == "" {
		t.Fatalf("source_info = %+v, want PULL_FROM_URL", body.SourceInfo)
	}
	if body.PostInfo.PrivacyLevel != "SELF_ONLY" || body.PostInfo.Title != "hi" {
		t.Fatalf("post_info = %+v", body.PostInfo)
	}
	if !strings.Contains(stdout, "v_pub_1") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPostVideoDirectRequiresPrivacy(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "video", "--url", "https://x/v.mp4")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--privacy") {
		t.Fatalf("stderr = %q, want privacy-required error", stderr)
	}
}

func TestPostVideoRejectsBothSources(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"post", "video", "--url", "https://x/v.mp4", "--file", "/tmp/v.mp4", "--privacy", "SELF_ONLY")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestPostVideoDraftUsesInbox(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"publish_id":"v_inbox_1"}`))
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"post", "video", "--url", "https://cdn.example.com/v.mp4", "--draft")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v2/post/publish/inbox/video/init/" {
		t.Fatalf("path = %q, want inbox init", got.Path)
	}
	// Draft uploads carry no post_info.
	if strings.Contains(string(got.Body), "post_info") {
		t.Fatalf("draft body carried post_info: %s", got.Body)
	}
}

func TestPostVideoFileUploadTwoStep(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "clip.mp4")
	contents := []byte("fake-mp4-bytes")
	if err := os.WriteFile(videoPath, contents, 0o600); err != nil {
		t.Fatalf("write temp video: %v", err)
	}

	var initReq, uploadReq capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			uploadReq = captureRequest(t, r)
			w.WriteHeader(http.StatusCreated)
			return
		}
		initReq = captureRequest(t, r)
		// upload_url points back at this same test server.
		jsonResponse(w, http.StatusOK, okEnvelope(`{"publish_id":"v_file_1","upload_url":"`+serverURL(r)+`/upload"}`))
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(),
		"post", "video", "--file", videoPath, "--privacy", "SELF_ONLY")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}

	var body struct {
		SourceInfo struct {
			Source          string `json:"source"`
			VideoSize       int64  `json:"video_size"`
			TotalChunkCount int    `json:"total_chunk_count"`
		} `json:"source_info"`
	}
	if err := json.Unmarshal(initReq.Body, &body); err != nil {
		t.Fatalf("decode init body: %v (%s)", err, initReq.Body)
	}
	if body.SourceInfo.Source != "FILE_UPLOAD" || body.SourceInfo.VideoSize != int64(len(contents)) {
		t.Fatalf("source_info = %+v, want FILE_UPLOAD size %d", body.SourceInfo, len(contents))
	}
	if uploadReq.Method != http.MethodPut {
		t.Fatalf("no PUT upload happened")
	}
	wantRange := "bytes 0-13/14"
	if uploadReq.rangeHeader != wantRange {
		t.Fatalf("Content-Range = %q, want %q", uploadReq.rangeHeader, wantRange)
	}
	if string(uploadReq.Body) != string(contents) {
		t.Fatalf("uploaded body = %q, want %q", uploadReq.Body, contents)
	}
	if !strings.Contains(stdout, "v_file_1") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPostStatusRequest(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"status":"PROCESSING_UPLOAD"}`))
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "post", "status", "--publish-id", "v_pub_1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v2/post/publish/status/fetch/" {
		t.Fatalf("path = %q", got.Path)
	}
	if !strings.Contains(string(got.Body), `"publish_id":"v_pub_1"`) {
		t.Fatalf("body = %s", got.Body)
	}
	if !strings.Contains(stdout, "PROCESSING_UPLOAD") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPostStatusRequiresPublishID(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "status")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--publish-id is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}
