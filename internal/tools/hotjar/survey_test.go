package hotjar

import (
	"net/http"
	"strings"
	"testing"
)

func TestSurveyGet_WithQuestionsFlag(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/sites/42/surveys/7", cannedResponse{http.StatusOK, `{"id":7,"name":"NPS"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runHotjar(t, srv, "survey", "get", "--site", "42", "--survey", "7", "--with-questions")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	req := fake.first(t, "/v1/sites/42/surveys/7")
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if q := parseQuery(t, req.Query); q.Get("with_questions") != "true" {
		t.Errorf("with_questions = %q, want true", q.Get("with_questions"))
	}
	if !strings.Contains(stdout, `"name":"NPS"`) {
		t.Errorf("stdout missing payload: %q", stdout)
	}
}

func TestSurveyResponses_PathAndPagination(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/sites/42/surveys/7/responses", cannedResponse{http.StatusOK,
			`{"results":[{"id":1}],"next_cursor":"c9"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runHotjar(t, srv, "survey", "responses", "--site", "42", "--survey", "7", "--cursor", "c8", "--limit", "10")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	req := fake.first(t, "/v1/sites/42/surveys/7/responses")
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if req.Auth != "Bearer tok-abc" {
		t.Errorf("auth = %q, want Bearer tok-abc", req.Auth)
	}
	q := parseQuery(t, req.Query)
	if q.Get("cursor") != "c8" || q.Get("limit") != "10" {
		t.Errorf("pagination params not forwarded: %v", q)
	}
	if !strings.Contains(stdout, `"next_cursor":"c9"`) {
		t.Errorf("stdout missing passthrough payload: %q", stdout)
	}
}

func TestSurveyResponses_MissingRequiredFlagsIsUsageError(t *testing.T) {
	srv := newFake().withToken().serve(t)
	defer srv.Close()

	// --survey omitted → cobra required-flag error → exit 2 (usage), no token
	// exchange attempted.
	result, _, _ := runHotjar(t, srv, "survey", "responses", "--site", "42")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", result.ExitCode)
	}
}
