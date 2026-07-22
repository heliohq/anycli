package tiktok

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestVideoListRequest(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"videos":[{"id":"1"}],"cursor":100,"has_more":false}`))
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "video", "list", "--max-count", "5", "--cursor", "42")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/video/list/" {
		t.Fatalf("request = %s %s, want POST /v2/video/list/", got.Method, got.Path)
	}
	if !strings.Contains(got.RawQuery, "fields=") {
		t.Fatalf("query = %q, want fields param", got.RawQuery)
	}
	var body map[string]any
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, got.Body)
	}
	if body["max_count"].(float64) != 5 || body["cursor"].(float64) != 42 {
		t.Fatalf("body = %v, want max_count 5 and cursor 42", body)
	}
	if !strings.Contains(stdout, `"has_more":false`) {
		t.Fatalf("stdout = %q, want the data object", stdout)
	}
}

func TestVideoListRejectsOutOfRangeMaxCount(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "video", "list", "--max-count", "50")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--max-count must be between 1 and 20") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestVideoQueryRequiresIDs(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "video", "query")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--ids is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestVideoQueryBuildsFilters(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, okEnvelope(`{"videos":[]}`))
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "video", "query", "--ids", "a, b ,c")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v2/video/query/" {
		t.Fatalf("path = %q, want /v2/video/query/", got.Path)
	}
	var body struct {
		Filters struct {
			VideoIDs []string `json:"video_ids"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, got.Body)
	}
	if len(body.Filters.VideoIDs) != 3 || body.Filters.VideoIDs[0] != "a" || body.Filters.VideoIDs[2] != "c" {
		t.Fatalf("video_ids = %v, want [a b c] trimmed", body.Filters.VideoIDs)
	}
}
