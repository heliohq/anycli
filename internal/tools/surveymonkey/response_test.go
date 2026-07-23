package surveymonkey

import (
	"net/http"
	"strings"
	"testing"
)

func TestResponseList(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[{"id":"9","href":"h"}],"total":1}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(),
		"response", "list", "--survey", "42",
		"--status", "completed", "--page", "1", "--per-page", "10",
		"--start-modified-at", "2026-01-01T00:00:00")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/surveys/42/responses" {
		t.Fatalf("path = %q, want /v3/surveys/42/responses (non-bulk)", got.Path)
	}
	if !strings.Contains(got.RawQuery, "status=completed") {
		t.Fatalf("query = %q, want status=completed", got.RawQuery)
	}
	if !strings.Contains(got.RawQuery, "start_modified_at=") {
		t.Fatalf("query = %q, want start_modified_at filter", got.RawQuery)
	}
	if !strings.Contains(stdout, `"href":"h"`) {
		t.Fatalf("stdout = %q, want metadata list", stdout)
	}
}

func TestResponseListRequiresSurvey(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "response", "list")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "survey") {
		t.Fatalf("stderr = %q, want required-flag error", stderr)
	}
}

func TestResponseBulk(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[{"id":"9","pages":[]}],"total":1}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"response", "bulk", "--survey", "42", "--status", "completed", "--per-page", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/surveys/42/responses/bulk" {
		t.Fatalf("path = %q, want /v3/surveys/42/responses/bulk", got.Path)
	}
	if !strings.Contains(got.RawQuery, "status=completed") || !strings.Contains(got.RawQuery, "per_page=5") {
		t.Fatalf("query = %q, want status & per_page", got.RawQuery)
	}
}

func TestResponseGet(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"id":"9","pages":[{"questions":[]}]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "response", "get", "--survey", "42", "--id", "9")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/surveys/42/responses/9/details" {
		t.Fatalf("path = %q, want /v3/surveys/42/responses/9/details", got.Path)
	}
}

func TestResponseGetRequiresBothFlags(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, _ := run(t, server, fullEnv(), "response", "get", "--survey", "42")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage) when --id missing", code)
	}
}
