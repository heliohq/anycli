package snov

import (
	"net/http"
	"strings"
	"testing"
)

func TestEmailCount_V1Form(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/get-domain-emails-count", cannedResponse{http.StatusOK, `{"success":true,"domain":"example.com","result":42}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runSnov(t, srv, "email", "count", "--domain", "example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	req := fake.first(t, "/v1/get-domain-emails-count")
	if req.Method != http.MethodPost {
		t.Errorf("count method = %s, want POST", req.Method)
	}
	form := parseForm(t, req.Body)
	if form.Get("domain") != "example.com" {
		t.Errorf("domain = %q, want example.com", form.Get("domain"))
	}
	if form.Get("access_token") != "tok-abc" {
		t.Errorf("count access_token = %q, want tok-abc", form.Get("access_token"))
	}
	if !strings.Contains(stdout, `"result":42`) {
		t.Errorf("stdout missing count payload: %q", stdout)
	}
}

func TestEmailVerify_AsyncStartThenPollUntilComplete(t *testing.T) {
	fake := newFake().withToken().
		on("/v2/email-verification/start", cannedResponse{http.StatusOK, `{"task_hash":"h123","emails":["a@example.com"]}`}).
		on("/v2/email-verification/result",
			cannedResponse{http.StatusOK, `{"status":"in progress"}`},
			cannedResponse{http.StatusOK, `{"status":"completed","email":"a@example.com","smtp_status":"valid"}`},
		)
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runSnov(t, srv, "email", "verify", "--email", "a@example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	// start uses Bearer auth + JSON body carrying emails[].
	start := fake.first(t, "/v2/email-verification/start")
	if start.Auth != "Bearer tok-abc" {
		t.Errorf("start Auth = %q, want Bearer tok-abc", start.Auth)
	}
	if !strings.Contains(start.ContentType, "application/json") {
		t.Errorf("start Content-Type = %q, want json", start.ContentType)
	}
	body := decodeJSON(t, start.Body)
	emails, ok := body["emails"].([]any)
	if !ok || len(emails) != 1 || emails[0] != "a@example.com" {
		t.Errorf("start body emails = %v, want [a@example.com]", body["emails"])
	}
	// result was polled with the task_hash query until status=completed.
	results := fake.hits("/v2/email-verification/result")
	if len(results) < 2 {
		t.Fatalf("expected at least 2 result polls, got %d", len(results))
	}
	if got := parseQuery(t, results[0].Query).Get("task_hash"); got != "h123" {
		t.Errorf("result task_hash = %q, want h123", got)
	}
	if results[0].Auth != "Bearer tok-abc" {
		t.Errorf("result Auth = %q, want Bearer", results[0].Auth)
	}
	// Only the completed payload is emitted, never the intermediate task.
	if !strings.Contains(stdout, `"smtp_status":"valid"`) {
		t.Errorf("stdout missing completed payload: %q", stdout)
	}
	if strings.Contains(stdout, "in progress") {
		t.Errorf("stdout leaked the in-progress task: %q", stdout)
	}
}

func TestEmailFindDomain_PollsLinksResultURL(t *testing.T) {
	fake := newFake().withToken()
	srv := fake.serve(t)
	defer srv.Close()
	// The start reply points at a full result URL on the same fake host.
	resultURL := srv.URL + "/v2/domain-search/domain-emails/result/h999"
	fake.on("/v2/domain-search/domain-emails/start",
		cannedResponse{http.StatusOK, `{"meta":{"task_hash":"h999"},"links":{"result":"` + resultURL + `"}}`}).
		on("/v2/domain-search/domain-emails/result/h999",
			cannedResponse{http.StatusOK, `{"status":"in progress"}`},
			cannedResponse{http.StatusOK, `{"status":"completed","data":[{"email":"ceo@example.com"}]}`},
		)

	result, stdout, stderr := runSnov(t, srv, "email", "find", "domain", "--domain", "example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	start := fake.first(t, "/v2/domain-search/domain-emails/start")
	if got := decodeJSON(t, start.Body)["domain"]; got != "example.com" {
		t.Errorf("start domain = %v, want example.com", got)
	}
	// Polling followed the server-provided links.result URL (path form), not a
	// query-param result endpoint.
	if hits := fake.hits("/v2/domain-search/domain-emails/result/h999"); len(hits) < 2 {
		t.Fatalf("expected polling on the links.result URL, got %d hits", len(hits))
	}
	if !strings.Contains(stdout, "ceo@example.com") {
		t.Errorf("stdout missing completed payload: %q", stdout)
	}
}

func TestEmailFindByName_SendsRows(t *testing.T) {
	fake := newFake().withToken().
		on("/v2/emails-by-domain-by-name/start", cannedResponse{http.StatusOK, `{"task_hash":"hn1"}`}).
		on("/v2/emails-by-domain-by-name/result",
			cannedResponse{http.StatusOK, `{"status":"completed","email":"jane.doe@example.com","smtp_status":"valid"}`},
		)
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runSnov(t, srv, "email", "find", "by-name",
		"--first", "Jane", "--last", "Doe", "--domain", "example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	start := fake.first(t, "/v2/emails-by-domain-by-name/start")
	body := decodeJSON(t, start.Body)
	rows, ok := body["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("start body rows = %v, want one row", body["rows"])
	}
	row := rows[0].(map[string]any)
	if row["first_name"] != "Jane" || row["last_name"] != "Doe" || row["domain"] != "example.com" {
		t.Errorf("row = %v, want Jane/Doe/example.com", row)
	}
	if !strings.Contains(stdout, "jane.doe@example.com") {
		t.Errorf("stdout missing completed payload: %q", stdout)
	}
}

func TestEmailVerify_ResultAuthErrorRejectsCredential(t *testing.T) {
	fake := newFake().withToken().
		on("/v2/email-verification/start", cannedResponse{http.StatusOK, `{"task_hash":"h1"}`}).
		on("/v2/email-verification/result", cannedResponse{http.StatusUnauthorized, `{"message":"token expired"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, _ := runSnov(t, srv, "email", "verify", "--email", "a@example.com")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 with credential rejection", result)
	}
}

func TestEmailVerify_PollTimeout(t *testing.T) {
	fake := newFake().withToken().
		on("/v2/email-verification/start", cannedResponse{http.StatusOK, `{"task_hash":"h1"}`}).
		on("/v2/email-verification/result", cannedResponse{http.StatusOK, `{"status":"in progress"}`})
	srv := fake.serve(t)
	defer srv.Close()

	// A tiny --timeout forces the never-completing task to give up non-fatally.
	result, _, stderr := runSnov(t, srv, "email", "verify", "--email", "a@example.com", "--timeout", "5ms")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("a timeout is not a credential rejection")
	}
	if !strings.Contains(stderr, "did not complete") {
		t.Errorf("stderr should explain the timeout: %q", stderr)
	}
}
