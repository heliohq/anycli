package close

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecute_MissingToken(t *testing.T) {
	code, _, stderr := run(t, nil, map[string]string{}, "me")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "CLOSE_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", stderr)
	}
}

func TestMe_Happy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me/": {status: 200, body: `{"id":"user_1","email":"a@b.com","organizations":[{"id":"orga_1","name":"Acme"}]}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, fullEnv(), "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodGet, "/me/")
	if got == nil {
		t.Fatalf("no GET /me/ request; saw %+v", reqs)
	}
	if got.Auth != "Bearer close-token" {
		t.Errorf("Authorization = %q, want Bearer close-token", got.Auth)
	}
	if !strings.Contains(stdout, `"id":"user_1"`) {
		t.Errorf("stdout = %q, want the provider JSON verbatim", stdout)
	}
}

func TestLeadList_PaginationFlags(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /lead/": {status: 200, body: `{"data":[{"id":"lead_1"}],"has_more":false}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, fullEnv(), "lead", "list", "--limit", "25", "--skip", "50")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodGet, "/lead/")
	if got == nil {
		t.Fatalf("no GET /lead/ request; saw %+v", reqs)
	}
	if got.Query.Get("_limit") != "25" || got.Query.Get("_skip") != "50" {
		t.Errorf("query = %v, want _limit=25 _skip=50", got.Query)
	}
	if !strings.Contains(stdout, `"has_more":false`) {
		t.Errorf("stdout = %q, want provider list envelope verbatim", stdout)
	}
}

func TestLeadGet_Happy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /lead/lead_9/": {status: 200, body: `{"id":"lead_9","display_name":"Widgets Inc"}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, fullEnv(), "lead", "get", "lead_9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/lead/lead_9/") == nil {
		t.Fatalf("no GET /lead/lead_9/ request; saw %+v", reqs)
	}
	if !strings.Contains(stdout, "Widgets Inc") {
		t.Errorf("stdout = %q, want the lead JSON", stdout)
	}
}

func TestLeadCreate_DataBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /lead/": {status: 200, body: `{"id":"lead_new"}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(), "lead", "create", "--data", `{"name":"Widgets Inc","url":"widgets.com"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodPost, "/lead/")
	if got == nil {
		t.Fatalf("no POST /lead/ request; saw %+v", reqs)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ContentType)
	}
	payload := bodyMap(t, got.Body)
	if payload["name"] != "Widgets Inc" || payload["url"] != "widgets.com" {
		t.Errorf("body = %v, want the --data JSON forwarded verbatim", payload)
	}
}

func TestLeadCreate_DataFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "lead.json")
	if err := os.WriteFile(file, []byte(`{"name":"From File"}`), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /lead/": {status: 200, body: `{"id":"lead_f"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(), "lead", "create", "--data", "@"+file)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%s)", code, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/lead/")
	if got == nil {
		t.Fatalf("no POST /lead/ request; saw %+v", reqs)
	}
	if bodyMap(t, got.Body)["name"] != "From File" {
		t.Errorf("body = %s, want the @file JSON", got.Body)
	}
}

func TestLeadUpdate_PutData(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /lead/lead_9/": {status: 200, body: `{"id":"lead_9"}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(), "lead", "update", "lead_9", "--data", `{"status_id":"stat_1"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodPut, "/lead/lead_9/")
	if got == nil {
		t.Fatalf("no PUT /lead/lead_9/ request; saw %+v", reqs)
	}
	if bodyMap(t, got.Body)["status_id"] != "stat_1" {
		t.Errorf("body = %s, want the update JSON", got.Body)
	}
}

func TestLeadDelete_Happy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"DELETE /lead/lead_9/": {status: 200, body: `{}`},
	})
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(), "lead", "delete", "lead_9")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if findReq(reqs, http.MethodDelete, "/lead/lead_9/") == nil {
		t.Fatalf("no DELETE /lead/lead_9/ request; saw %+v", reqs)
	}
}

// Contact and opportunity share the generic CRUD builder; a representative
// path per resource proves the builder wired the right collection path.
func TestContactCreate_Path(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"POST /contact/": {status: 200, body: `{"id":"cont_1"}`}})
	defer srv.Close()
	code, _, _ := run(t, srv, fullEnv(), "contact", "create", "--data", `{"lead_id":"lead_1","name":"Jane"}`)
	if code != 0 || findReq(reqs, http.MethodPost, "/contact/") == nil {
		t.Fatalf("contact create failed: code=%d reqs=%+v", code, reqs)
	}
}

func TestOpportunityGet_Path(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /opportunity/oppo_1/": {status: 200, body: `{"id":"oppo_1"}`}})
	defer srv.Close()
	code, _, _ := run(t, srv, fullEnv(), "opportunity", "get", "oppo_1")
	if code != 0 || findReq(reqs, http.MethodGet, "/opportunity/oppo_1/") == nil {
		t.Fatalf("opportunity get failed: code=%d reqs=%+v", code, reqs)
	}
}

func TestTaskComplete_PutIsComplete(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"PUT /task/task_1/": {status: 200, body: `{"id":"task_1","is_complete":true}`}})
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(), "task", "complete", "task_1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodPut, "/task/task_1/")
	if got == nil {
		t.Fatalf("no PUT /task/task_1/ request; saw %+v", reqs)
	}
	if bodyMap(t, got.Body)["is_complete"] != true {
		t.Errorf("body = %s, want is_complete=true", got.Body)
	}
}

func TestActivityList_Filters(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /activity/": {status: 200, body: `{"data":[],"has_more":false}`}})
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(), "activity", "list", "--lead-id", "lead_1", "--type", "Note", "--limit", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodGet, "/activity/")
	if got == nil {
		t.Fatalf("no GET /activity/ request; saw %+v", reqs)
	}
	if got.Query.Get("lead_id") != "lead_1" || got.Query.Get("_type") != "Note" || got.Query.Get("_limit") != "10" {
		t.Errorf("query = %v, want lead_id=lead_1 _type=Note _limit=10", got.Query)
	}
}

func TestActivityNoteAdd_Body(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"POST /activity/note/": {status: 200, body: `{"id":"acti_1"}`}})
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(), "activity", "note-add", "--lead-id", "lead_1", "--note", "Called and left a voicemail")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodPost, "/activity/note/")
	if got == nil {
		t.Fatalf("no POST /activity/note/ request; saw %+v", reqs)
	}
	payload := bodyMap(t, got.Body)
	if payload["lead_id"] != "lead_1" || payload["note"] != "Called and left a voicemail" {
		t.Errorf("body = %v, want lead_id + note", payload)
	}
}

func TestActivityCreate_GenericTypeData(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"POST /activity/call/": {status: 200, body: `{"id":"acti_call"}`}})
	defer srv.Close()

	code, _, _ := run(t, srv, fullEnv(), "activity", "create", "call", "--data", `{"lead_id":"lead_1","direction":"outbound","note":"quick sync"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodPost, "/activity/call/")
	if got == nil {
		t.Fatalf("no POST /activity/call/ request; saw %+v", reqs)
	}
	if bodyMap(t, got.Body)["direction"] != "outbound" {
		t.Errorf("body = %s, want the raw activity JSON", got.Body)
	}
}

func TestActivityGetAndDelete_TypedPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /activity/note/acti_1/":    {status: 200, body: `{"id":"acti_1"}`},
		"DELETE /activity/note/acti_1/": {status: 200, body: `{}`},
	})
	defer srv.Close()

	if code, _, _ := run(t, srv, fullEnv(), "activity", "get", "note", "acti_1"); code != 0 {
		t.Fatalf("activity get exit = %d, want 0", code)
	}
	if code, _, _ := run(t, srv, fullEnv(), "activity", "delete", "note", "acti_1"); code != 0 {
		t.Fatalf("activity delete exit = %d, want 0", code)
	}
	if findReq(reqs, http.MethodGet, "/activity/note/acti_1/") == nil {
		t.Errorf("no GET /activity/note/acti_1/; saw %+v", reqs)
	}
	if findReq(reqs, http.MethodDelete, "/activity/note/acti_1/") == nil {
		t.Errorf("no DELETE /activity/note/acti_1/; saw %+v", reqs)
	}
}

func TestSearch_PostDataBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"POST /data/search/": {status: 200, body: `{"data":[{"id":"lead_1"}],"cursor":null}`}})
	defer srv.Close()

	query := `{"query":{"type":"object_type","object_type":"lead"},"_limit":50}`
	code, stdout, _ := run(t, srv, fullEnv(), "search", "--data", query)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := findReq(reqs, http.MethodPost, "/data/search/")
	if got == nil {
		t.Fatalf("no POST /data/search/ request; saw %+v", reqs)
	}
	payload := bodyMap(t, got.Body)
	if _, ok := payload["query"]; !ok {
		t.Errorf("body = %s, want the advanced-filtering query forwarded", got.Body)
	}
	if !strings.Contains(stdout, `"id":"lead_1"`) {
		t.Errorf("stdout = %q, want the search results verbatim", stdout)
	}
}

// --- error / exit-code contract ---

func TestAPIError_ExitOneAndMessage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /lead/lead_x/": {status: 400, body: `{"error":"invalid lead id","field-errors":{"id":"bad"}}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(), "lead", "get", "lead_x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for API error", code)
	}
	if !strings.Contains(stderr, "invalid lead id") {
		t.Errorf("stderr = %q, want the Close error message", stderr)
	}
}

func TestAPIError_JSONEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /lead/lead_x/": {status: 400, body: `{"error":"invalid lead id"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, fullEnv(), "--json", "lead", "get", "lead_x")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, `"kind":"api"`) || !strings.Contains(stderr, `"status":400`) {
		t.Errorf("stderr = %q, want JSON error envelope with kind=api status=400", stderr)
	}
}

func TestUsageError_ExitTwo(t *testing.T) {
	// Unknown subcommand is a usage error → exit 2.
	code, _, _ := run(t, nil, fullEnv(), "lead", "frobnicate")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for usage error", code)
	}
}

func TestCreate_MissingData_UsageError(t *testing.T) {
	// create with no --data is a usage error → exit 2, no request sent.
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	code, _, _ := run(t, srv, fullEnv(), "lead", "create")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 when --data is missing", code)
	}
	if len(reqs) != 0 {
		t.Errorf("no request must be sent without --data, saw %+v", reqs)
	}
}

func TestBadDataJSON_UsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	code, _, stderr := run(t, srv, fullEnv(), "lead", "create", "--data", `{not json`)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for invalid JSON", code)
	}
	if !strings.Contains(stderr, "not valid JSON") {
		t.Errorf("stderr = %q, want an invalid-JSON message", stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("no request must be sent for invalid JSON, saw %+v", reqs)
	}
}

func TestCredentialRejection_On401(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server error", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{
				"GET /me/": {status: tc.status, body: `{"error":"nope"}`},
			})
			defer srv.Close()

			result, _, _ := runResult(t, srv, fullEnv(), "me")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
		})
	}
}
