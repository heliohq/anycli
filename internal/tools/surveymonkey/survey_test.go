package surveymonkey

import (
	"net/http"
	"strings"
	"testing"
)

func TestSurveyList(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[{"id":"1","title":"NPS"}],"page":1,"per_page":50,"total":1}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "survey", "list", "--page", "2", "--per-page", "25")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v3/surveys" {
		t.Fatalf("request = %s %s, want GET /v3/surveys", got.Method, got.Path)
	}
	if got.Auth != "Bearer sm-access-token" {
		t.Fatalf("auth = %q, want Bearer sm-access-token", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Fatalf("accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(got.RawQuery, "page=2") || !strings.Contains(got.RawQuery, "per_page=25") {
		t.Fatalf("query = %q, want page=2 & per_page=25", got.RawQuery)
	}
	if !strings.Contains(stdout, `"title":"NPS"`) {
		t.Fatalf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestSurveyListDefaultsNoPagingParams(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "survey", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.RawQuery != "" {
		t.Fatalf("query = %q, want empty when no paging flags", got.RawQuery)
	}
}

func TestSurveyGet(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"id":"123","title":"NPS","response_count":42}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "survey", "get", "--id", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/surveys/123" {
		t.Fatalf("path = %q, want /v3/surveys/123", got.Path)
	}
	if !strings.Contains(stdout, `"response_count":42`) {
		t.Fatalf("stdout = %q, want survey JSON", stdout)
	}
}

func TestSurveyGetRequiresID(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "survey", "get")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "id") {
		t.Fatalf("stderr = %q, want required-flag error", stderr)
	}
}

func TestSurveyDetails(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"id":"123","pages":[{"questions":[]}]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "survey", "details", "--id", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/surveys/123/details" {
		t.Fatalf("path = %q, want /v3/surveys/123/details", got.Path)
	}
}
