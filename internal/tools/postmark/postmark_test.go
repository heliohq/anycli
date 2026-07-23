package postmark

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// sendOK is a Postmark single-send success body: HTTP 200 with ErrorCode 0.
const sendOK = `{"To":"a@b.com","SubmittedAt":"2026-07-22T00:00:00Z","MessageID":"m-1","ErrorCode":0,"Message":"OK"}`

func TestEmailSendInjectsServerTokenAndHeaders(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, sendOK, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv,
		"email", "send", "--from", "s@x.com", "--to", "r@y.com", "--subject", "Hi", "--text", "body")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.ServerToken != testToken {
		t.Errorf("X-Postmark-Server-Token = %q, want %q", got.ServerToken, testToken)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json (write request)", got.ContentType)
	}
	if got.Method != "POST" || got.Path != "/email" {
		t.Errorf("request = %s %s, want POST /email", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["From"] != "s@x.com" || body["To"] != "r@y.com" || body["Subject"] != "Hi" || body["TextBody"] != "body" {
		t.Errorf("unexpected send body: %v", body)
	}
	if !strings.Contains(stdout, `"MessageID":"m-1"`) {
		t.Errorf("stdout missing provider body: %q", stdout)
	}
}

func TestEmailSendValidationErrorSurfacesMessage(t *testing.T) {
	// A 422 with a non-zero ErrorCode (406 = inactive recipient) → exit 1.
	var got capturedRequest
	srv := newServer(t, 422, `{"ErrorCode":406,"Message":"You tried to send to a recipient that has been marked as inactive."}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "email", "send", "--from", "s@x.com", "--to", "r@y.com", "--text", "hi")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on error, got %q", stdout)
	}
	if !strings.Contains(stderr, "marked as inactive") {
		t.Errorf("stderr missing provider Message: %q", stderr)
	}
}

func TestReadSuccessWithoutErrorCodeField(t *testing.T) {
	// A successful read has no ErrorCode field at all; the zero-value 0 must be
	// treated as success (the "absent" arm of the success key).
	var got capturedRequest
	srv := newServer(t, 200, `{"TotalCount":1,"Templates":[{"TemplateId":7,"Alias":"welcome"}]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "template", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != "GET" || got.Path != "/templates" {
		t.Errorf("request = %s %s, want GET /templates", got.Method, got.Path)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("count") != "100" || q.Get("offset") != "0" {
		t.Errorf("template list query = %q, want count=100 offset=0", got.Query)
	}
	if !strings.Contains(stdout, `"Alias":"welcome"`) {
		t.Errorf("stdout missing body: %q", stdout)
	}
}

func TestBadTokenRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"ErrorCode":10,"Message":"No Account or Server API tokens were supplied."}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "server", "get")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true for a 401")
	}
	if !strings.Contains(stderr, "HTTP 401") {
		t.Errorf("stderr missing status: %q", stderr)
	}
}

func TestServerGetRedactsApiTokens(t *testing.T) {
	// GET /server echoes the caller's token in ApiTokens; `server get` must
	// never print it.
	body := `{"ID":42,"Name":"My Server","ApiTokens":["super-secret-server-token"],"Color":"Blue","ServerLink":"https://postmarkapp.com/servers/42/streams","DeliveryType":"Live","InboundAddress":"abc@inbound.postmarkapp.com"}`
	var got capturedRequest
	srv := newServer(t, 200, body, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "server", "get")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if strings.Contains(stdout, "super-secret-server-token") || strings.Contains(stdout, "ApiTokens") {
		t.Fatalf("server get leaked ApiTokens into stdout: %q", stdout)
	}
	var view map[string]any
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout)
	}
	if view["Name"] != "My Server" || view["ServerLink"] != "https://postmarkapp.com/servers/42/streams" {
		t.Errorf("redacted view missing safe fields: %v", view)
	}
	if _, ok := view["ApiTokens"]; ok {
		t.Errorf("redacted view still carries ApiTokens: %v", view)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 422, `{"ErrorCode":300,"Message":"Invalid email request."}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "--json", "email", "send", "--from", "s@x.com", "--to", "r@y.com", "--text", "hi")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Message   string `json:"message"`
			Kind      string `json:"kind"`
			Status    int    `json:"status"`
			ErrorCode int    `json:"error_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 || env.Error.ErrorCode != 300 {
		t.Errorf("envelope = %+v, want kind=api status=422 error_code=300", env.Error)
	}
}

func TestSendTemplateRequestShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, sendOK, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"email", "send-template", "--from", "s@x.com", "--to", "r@y.com",
		"--template-alias", "welcome", "--model", `{"name":"Ada"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/email/withTemplate" {
		t.Errorf("path = %q, want /email/withTemplate", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["TemplateAlias"] != "welcome" {
		t.Errorf("TemplateAlias = %v, want welcome", body["TemplateAlias"])
	}
	model, ok := body["TemplateModel"].(map[string]any)
	if !ok || model["name"] != "Ada" {
		t.Errorf("TemplateModel = %v, want {name:Ada}", body["TemplateModel"])
	}
}

func TestSendTemplateRequiresExactlyOneSelector(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, sendOK, &got)
	defer srv.Close()

	// Neither selector → usage error, exit 2.
	exit, _, _ := run(t, srv, "email", "send-template", "--from", "s@x.com", "--to", "r@y.com")
	if exit != 2 {
		t.Errorf("no selector: exit = %d, want 2", exit)
	}
	// Both selectors → usage error, exit 2.
	exit, _, _ = run(t, srv,
		"email", "send-template", "--from", "s@x.com", "--to", "r@y.com",
		"--template-id", "7", "--template-alias", "welcome")
	if exit != 2 {
		t.Errorf("both selectors: exit = %d, want 2", exit)
	}
}

func TestMessageListOutboundQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"TotalCount":0,"Messages":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"message", "list-outbound", "--count", "25", "--offset", "50",
		"--recipient", "r@y.com", "--tag", "welcome", "--stream", "broadcast")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	q, _ := url.ParseQuery(got.Query)
	if q.Get("count") != "25" || q.Get("offset") != "50" {
		t.Errorf("paging query = %q", got.Query)
	}
	if q.Get("recipient") != "r@y.com" || q.Get("tag") != "welcome" || q.Get("messagestream") != "broadcast" {
		t.Errorf("filter query = %q", got.Query)
	}
}

func TestBounceActivateUsesPut(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"Message":"OK","Bounce":{"ID":123,"Inactive":false}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "bounce", "activate", "123")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != "PUT" || got.Path != "/bounces/123/activate" {
		t.Errorf("request = %s %s, want PUT /bounces/123/activate", got.Method, got.Path)
	}
}

func TestMissingRequiredFlagIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, sendOK, &got)
	defer srv.Close()

	// Missing --from → exit 2, and no request reaches the server.
	exit, _, _ := run(t, srv, "email", "send", "--to", "r@y.com", "--text", "hi")
	if exit != 2 {
		t.Errorf("exit = %d, want 2", exit)
	}
	if got.Method != "" {
		t.Errorf("request should not reach server on usage error, saw %s %s", got.Method, got.Path)
	}
}

func TestSendRequiresBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, sendOK, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "email", "send", "--from", "s@x.com", "--to", "r@y.com")
	if exit != 2 {
		t.Errorf("no --html/--text: exit = %d, want 2", exit)
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	svc := &Service{}
	var out, errBuf strings.Builder
	svc.Out = &out
	svc.Err = &errBuf
	result, err := svc.Execute(t.Context(), []string{"server", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "POSTMARK_SERVER_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token message", errBuf.String())
	}
}
