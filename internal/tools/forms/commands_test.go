package forms

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreate_AlwaysUnpublished(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/forms": {http.StatusOK, `{"formId":"newf","info":{"title":"Survey"}}`},
	})
	stdout := f.runOK(t, "create", "--title", "Survey", "--document-title", "Doc")
	got := f.last(t, "POST", "/v1/forms")
	if !strings.Contains(got.Query, "unpublished=true") {
		t.Errorf("query = %q, want unpublished=true", got.Query)
	}
	if !strings.Contains(string(got.Body), `"title":"Survey"`) || !strings.Contains(string(got.Body), `"documentTitle":"Doc"`) {
		t.Errorf("body = %q, want title + documentTitle", got.Body)
	}
	if !strings.Contains(stdout, "created unpublished form newf") {
		t.Errorf("stdout = %q, want the created summary", stdout)
	}
}

func TestBatchUpdate_ArrayNormalizedToRequestsBody(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/forms/f1:batchUpdate": {http.StatusOK, `{"form":{"formId":"f1"}}`},
	})
	reqs := `[{"createItem":{"item":{"title":"Q1","questionItem":{"question":{"choiceQuestion":{"type":"RADIO","options":[{"value":"A"}]}}}},"location":{"index":0}}}]`
	f.runOK(t, "batch-update", "f1", "--requests", reqs)
	got := f.last(t, "POST", "/v1/forms/f1:batchUpdate")
	if !strings.Contains(string(got.Body), `"requests":[`) {
		t.Errorf("body = %q, want a batchUpdate requests wrapper", got.Body)
	}
	if !strings.Contains(string(got.Body), `"createItem"`) {
		t.Errorf("body = %q, want the createItem request passed through", got.Body)
	}
}

func TestBatchUpdate_FullBodyPassedThrough(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/forms/f1:batchUpdate": {http.StatusOK, `{}`},
	})
	f.runOK(t, "batch-update", "f1", "--requests", `{"requests":[{"updateFormInfo":{"info":{"title":"X"},"updateMask":"title"}}]}`)
	got := f.last(t, "POST", "/v1/forms/f1:batchUpdate")
	if !strings.Contains(string(got.Body), `"updateFormInfo"`) {
		t.Errorf("body = %q, want the full body passed through", got.Body)
	}
}

func TestBatchUpdate_InvalidJSON(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "batch-update", "f1", "--requests", `{not json`)
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want an invalid-JSON error", stderr)
	}
	if len(f.requests) != 0 {
		t.Errorf("invalid JSON must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestPublishVerbs_MapToSetPublishSettings(t *testing.T) {
	cases := []struct {
		verb            string
		wantPublished   string
		wantAccepting   string
		wantHumanSubstr string
	}{
		{"publish", `"isPublished":true`, `"isAcceptingResponses":true`, "published form f1"},
		{"unpublish", `"isPublished":false`, `"isAcceptingResponses":false`, "unpublished form f1"},
		{"close", `"isPublished":true`, `"isAcceptingResponses":false`, "closed form f1"},
		{"reopen", `"isPublished":true`, `"isAcceptingResponses":true`, "reopened form f1"},
	}
	for _, tc := range cases {
		t.Run(tc.verb, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"POST /v1/forms/f1:setPublishSettings": {http.StatusOK, `{"publishSettings":{"publishState":{"isPublished":true,"isAcceptingResponses":true}}}`},
			})
			stdout := f.runOK(t, tc.verb, "f1")
			got := f.last(t, "POST", "/v1/forms/f1:setPublishSettings")
			body := string(got.Body)
			if !strings.Contains(body, tc.wantPublished) || !strings.Contains(body, tc.wantAccepting) {
				t.Errorf("%s body = %q, want %s + %s", tc.verb, body, tc.wantPublished, tc.wantAccepting)
			}
			if !strings.Contains(body, `"updateMask":"publishState"`) {
				t.Errorf("%s body = %q, want the publishState updateMask", tc.verb, body)
			}
			if !strings.Contains(stdout, tc.wantHumanSubstr) {
				t.Errorf("%s stdout = %q, want %q", tc.verb, stdout, tc.wantHumanSubstr)
			}
		})
	}
}

func TestResponsesList_HumanAndFilter(t *testing.T) {
	body := `{"responses":[{"responseId":"r1","createTime":"2026-07-01T00:00:00Z","respondentEmail":"a@b.c"}],"nextPageToken":"tok2"}`
	f := newFixture(t, map[string]route{
		"GET /v1/forms/f1/responses": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "responses", "list", "f1", "--filter", "timestamp >= 2026-01-01T00:00:00Z", "--max", "5")
	if !strings.Contains(stdout, "r1") || !strings.Contains(stdout, "next page token: tok2") {
		t.Errorf("stdout = %q, want the response id + next page token", stdout)
	}
	got := f.last(t, "GET", "/v1/forms/f1/responses")
	if !strings.Contains(got.Query, "filter=") || !strings.Contains(got.Query, "pageSize=5") {
		t.Errorf("query = %q, want the filter + pageSize passed through", got.Query)
	}
}

func TestResponsesGet(t *testing.T) {
	body := `{"responseId":"r1","createTime":"2026-07-01T00:00:00Z","answers":{"i1":{"textAnswers":{"answers":[{"value":"yes"}]}}}}`
	f := newFixture(t, map[string]route{
		"GET /v1/forms/f1/responses/r1": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "responses", "get", "f1", "r1")
	if !strings.Contains(stdout, "ResponseId: r1") || !strings.Contains(stdout, "yes") {
		t.Errorf("stdout = %q, want the response id + answer value", stdout)
	}
}
