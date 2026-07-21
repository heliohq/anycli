package novu

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAuthHeaderUsesApiKeyScheme is the load-bearing auth assertion: Novu
// requires the literal "ApiKey <secret>" scheme, NOT "Bearer".
func TestAuthHeaderUsesApiKeyScheme(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "integration", "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Auth != "ApiKey tok-123" {
		t.Errorf("Authorization = %q, want %q (literal ApiKey prefix, not Bearer)", got.Auth, "ApiKey tok-123")
	}
	if strings.HasPrefix(got.Auth, "Bearer") {
		t.Errorf("Authorization must not use Bearer scheme: %q", got.Auth)
	}
}

func TestEventTriggerPostsWorkflowAndRecipient(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{"acknowledged":true,"status":"processed","transactionId":"tx-1","activityFeedLink":"https://x"}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv,
		"event", "trigger", "--workflow", "welcome", "--to", "sub-1", "--payload", `{"name":"Ada"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/v1/events/trigger" {
		t.Fatalf("request = %s %s, want POST /v1/events/trigger", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "welcome" {
		t.Errorf("body.name = %v, want welcome", body["name"])
	}
	if body["to"] != "sub-1" {
		t.Errorf("body.to = %v, want sub-1", body["to"])
	}
	payload, ok := body["payload"].(map[string]any)
	if !ok || payload["name"] != "Ada" {
		t.Errorf("body.payload = %v, want {name:Ada}", body["payload"])
	}
	if !strings.Contains(stdout, `"status":"processed"`) {
		t.Errorf("stdout missing status field: %s", stdout)
	}
}

// TestEventTriggerSurfacesNonProcessedStatus guards the outcome semantics: an
// HTTP 201 with a non-processed status (trigger_not_active) means the send was
// accepted but NOT delivered. The tool must surface that status so an agent
// does not read 201 as success. (Exit stays 0 because the API call itself
// succeeded; the status field carries the real outcome.)
func TestEventTriggerSurfacesNonProcessedStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{"acknowledged":true,"status":"trigger_not_active","error":["Workflow is not active"],"transactionId":"tx-2"}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv,
		"event", "trigger", "--workflow", "welcome", "--to", "sub-1")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "trigger_not_active") {
		t.Errorf("stdout must surface the non-processed status, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Workflow is not active") {
		t.Errorf("stdout must surface the error[] detail, got: %s", stdout)
	}
}

func TestEventTriggerAcceptsTopicRecipientJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv,
		"event", "trigger", "--workflow", "digest", "--to-json", `{"type":"Topic","topicKey":"weekly"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	body := decodeBody(t, got.Body)
	to, ok := body["to"].(map[string]any)
	if !ok || to["topicKey"] != "weekly" {
		t.Errorf("body.to = %v, want topic object", body["to"])
	}
}

func TestEventTriggerRequiresRecipient(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "event", "trigger", "--workflow", "welcome")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "--to") {
		t.Errorf("stderr should mention the missing recipient flag: %s", stderr)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call should be made on a usage error, saw %s %s", got.Method, got.Path)
	}
}

func TestSubscriberListHitsV2WithFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"data":[],"next":null}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "subscriber", "list", "--email", "a@b.co", "--limit", "10")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/v2/subscribers" {
		t.Fatalf("request = %s %s, want GET /v2/subscribers", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("email") != "a@b.co" || q.Get("limit") != "10" {
		t.Errorf("query = %v, want email + limit", q)
	}
}

func TestSubscriberCreatePostsV2Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "subscriber", "create",
		"--subscriber-id", "sub-1", "--email", "a@b.co", "--first-name", "Ada")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/v2/subscribers" {
		t.Fatalf("request = %s %s, want POST /v2/subscribers", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["subscriberId"] != "sub-1" || body["email"] != "a@b.co" || body["firstName"] != "Ada" {
		t.Errorf("body = %v, want subscriberId+email+firstName", body)
	}
}

func TestTopicAddSubscribersPostsIDArray(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "topic", "add-subscribers", "--key", "weekly", "--subscriber-ids", "a, b ,c")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/v2/topics/weekly/subscriptions" {
		t.Fatalf("request = %s %s, want POST /v2/topics/weekly/subscriptions", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	ids, ok := body["subscriberIds"].([]any)
	if !ok || len(ids) != 3 || ids[0] != "a" || ids[2] != "c" {
		t.Errorf("body.subscriberIds = %v, want [a b c]", body["subscriberIds"])
	}
}

func TestWorkflowListHitsV2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"workflows":[],"totalCount":0}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "workflow", "list", "--limit", "5")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/v2/workflows" {
		t.Fatalf("request = %s %s, want GET /v2/workflows", got.Method, got.Path)
	}
}

func TestMessageListHitsV1WithQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[],"page":0,"pageSize":10,"hasMore":false}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "message", "list", "--channel", "email", "--limit", "10")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/v1/messages" {
		t.Fatalf("request = %s %s, want GET /v1/messages", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("channel") != "email" {
		t.Errorf("query.channel = %q, want email", q.Get("channel"))
	}
	// Pagination envelope passes through verbatim (no blanket data-unwrap).
	if !strings.Contains(stdout, `"hasMore":false`) {
		t.Errorf("stdout should preserve the pagination envelope: %s", stdout)
	}
}

func TestActivityListSendsRepeatedChannelParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[],"page":0,"pageSize":10,"hasMore":false}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "activity", "list", "--channels", "email,sms")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Path != "/v1/notifications" {
		t.Fatalf("path = %s, want /v1/notifications", got.Path)
	}
	q := parseQuery(t, got.Query)
	ch := q["channels"]
	if len(ch) != 2 || ch[0] != "email" || ch[1] != "sms" {
		t.Errorf("channels = %v, want repeated [email sms]", ch)
	}
}

func TestIntegrationListPassesBareArrayThrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"_id":"i1","channel":"email"}]`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "integration", "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got.Path != "/v1/integrations" {
		t.Fatalf("path = %s, want /v1/integrations", got.Path)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "[") {
		t.Errorf("bare array must pass through unchanged, got: %s", stdout)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"message":"Unauthorized","statusCode":401}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "subscriber", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("a 401 must set CredentialRejected so the token gateway invalidates the key")
	}
	if !strings.Contains(stderr, "Unauthorized") {
		t.Errorf("stderr should carry Novu's message: %s", stderr)
	}
}

func TestAPIErrorJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 422, `{"message":["name should not be empty"],"statusCode":422}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "--json", "event", "trigger", "--workflow", "x", "--to", "s")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("error envelope = %+v, want kind api status 422", env.Error)
	}
	if !strings.Contains(env.Error.Message, "name should not be empty") {
		t.Errorf("message should flatten validation array: %q", env.Error.Message)
	}
}

func TestUsageErrorJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "--json", "event", "trigger", "--to", "s")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, `"kind":"usage"`) {
		t.Errorf("usage error should render kind usage: %s", stderr)
	}
}

func TestMissingSecretFailsFast(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"integration", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvSecretKey) {
		t.Errorf("stderr should name the missing env var: %s", errBuf.String())
	}
}

func TestEURegionBaseFromEnv(t *testing.T) {
	// The region base travels via env (NOVU_API_BASE); with no BaseURL override,
	// Execute must read it. Point it at a fake server to prove it is used.
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	var out, errBuf strings.Builder
	svc := &Service{HC: srv.Client(), Out: &out, Err: &errBuf}
	env := map[string]string{EnvSecretKey: "tok-123", EnvAPIBase: srv.URL}
	result, err := svc.Execute(t.Context(), []string{"integration", "active"}, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", result.ExitCode, errBuf.String())
	}
	if got.Path != "/v1/integrations/active" {
		t.Errorf("path = %s, want /v1/integrations/active via NOVU_API_BASE", got.Path)
	}
}
