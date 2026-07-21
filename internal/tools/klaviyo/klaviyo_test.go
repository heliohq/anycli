package klaviyo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// --- core request shaping -------------------------------------------------

func TestAccountGetShapesRequest(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[{"type":"account","id":"AC1"}]}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "account", "get")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/accounts" {
		t.Errorf("request = %s %s, want GET /accounts", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Revision != apiRevision {
		t.Errorf("revision = %q, want %q", got.Revision, apiRevision)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if got.ContentType != "" {
		t.Errorf("GET must not send Content-Type, got %q", got.ContentType)
	}
	if !strings.Contains(stdout, `"id":"AC1"`) {
		t.Errorf("stdout = %q, want passthrough JSON:API body", stdout)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout must end with a newline, got %q", stdout)
	}
}

func TestPrivateKeyUsesKlaviyoAPIKeyScheme(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := runWithToken(t, srv, "pk_abc123", "account", "get")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Auth != "Klaviyo-API-Key pk_abc123" {
		t.Errorf("Authorization = %q, want Klaviyo-API-Key pk_abc123", got.Auth)
	}
}

// --- profiles -------------------------------------------------------------

func TestProfileListMapsSharedFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "list",
		"--filter", `equals(email,"x@y.com")`,
		"--sort", "-created",
		"--cursor", "CUR1",
		"--page-size", "50",
		"--include", "lists",
		"--fields", "email,first_name",
		"--param", "additional-fields[profile]=subscriptions")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	q := got.Query
	if q.Get("filter") != `equals(email,"x@y.com")` {
		t.Errorf("filter = %q", q.Get("filter"))
	}
	if q.Get("sort") != "-created" {
		t.Errorf("sort = %q", q.Get("sort"))
	}
	if q.Get("page[cursor]") != "CUR1" {
		t.Errorf("page[cursor] = %q", q.Get("page[cursor]"))
	}
	if q.Get("page[size]") != "50" {
		t.Errorf("page[size] = %q", q.Get("page[size]"))
	}
	if q.Get("include") != "lists" {
		t.Errorf("include = %q", q.Get("include"))
	}
	if q.Get("fields[profile]") != "email,first_name" {
		t.Errorf("fields[profile] = %q", q.Get("fields[profile]"))
	}
	if q.Get("additional-fields[profile]") != "subscriptions" {
		t.Errorf("additional-fields[profile] = %q", q.Get("additional-fields[profile]"))
	}
}

func TestPageSizeOutOfRangeIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "list", "--page-size", "101")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "page-size") {
		t.Errorf("stderr = %q, want page-size message", stderr)
	}
}

func TestProfileGetHitsResourcePath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"P1"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "profile", "get", "P1")
	if code != 0 || got.Path != "/profiles/P1" {
		t.Errorf("path = %q code=%d, want /profiles/P1", got.Path, code)
	}
}

func TestProfileCreateBuildsBodyFromFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{"id":"P9"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "create", "--email", "a@b.com", "--external-id", "ext1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/profiles" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ContentType)
	}
	m := bodyMap(t, got.Body)
	data := m["data"].(map[string]any)
	if data["type"] != "profile" {
		t.Errorf("type = %v", data["type"])
	}
	attrs := data["attributes"].(map[string]any)
	if attrs["email"] != "a@b.com" || attrs["external_id"] != "ext1" {
		t.Errorf("attributes = %v", attrs)
	}
	if _, ok := attrs["phone_number"]; ok {
		t.Errorf("empty phone must be omitted, got %v", attrs)
	}
}

func TestProfileCreateRequiresIdentifierOrData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "create")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--email") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestProfileUpdatePatchesWithID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"P1"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "update", "P1", "--email", "new@b.com")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "PATCH" || got.Path != "/profiles/P1" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	data := bodyMap(t, got.Body)["data"].(map[string]any)
	if data["id"] != "P1" {
		t.Errorf("id = %v, want P1", data["id"])
	}
}

func TestProfileCreateDataOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "create", "--email", "ignored@b.com",
		"--data", `{"data":{"type":"profile","attributes":{"email":"raw@b.com"}}}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	attrs := bodyMap(t, got.Body)["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["email"] != "raw@b.com" {
		t.Errorf("--data must win; email = %v", attrs["email"])
	}
}

// --- consent jobs ---------------------------------------------------------

func TestSubscribeBuildsConsentAndListRelationship(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{"data":{"id":"job1"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "subscribe", "--email", "a@b.com", "--list-id", "L1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Path != "/profile-subscription-bulk-create-jobs" {
		t.Errorf("path = %q", got.Path)
	}
	data := bodyMap(t, got.Body)["data"].(map[string]any)
	if data["type"] != "profile-subscription-bulk-create-job" {
		t.Errorf("type = %v", data["type"])
	}
	attrs := data["attributes"].(map[string]any)
	profiles := attrs["profiles"].(map[string]any)["data"].([]any)
	p0 := profiles[0].(map[string]any)["attributes"].(map[string]any)
	if p0["email"] != "a@b.com" {
		t.Errorf("profile email = %v", p0["email"])
	}
	subs := p0["subscriptions"].(map[string]any)
	if _, ok := subs["email"]; !ok {
		t.Errorf("email consent missing: %v", subs)
	}
	rel := data["relationships"].(map[string]any)["list"].(map[string]any)["data"].(map[string]any)
	if rel["id"] != "L1" || rel["type"] != "list" {
		t.Errorf("list relationship = %v", rel)
	}
}

func TestSubscribeSMSRequiresPhone(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "subscribe", "--email", "a@b.com", "--channel", "sms")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--phone") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestUnsubscribeOmitsConsent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "unsubscribe", "--email", "a@b.com")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Path != "/profile-subscription-bulk-delete-jobs" {
		t.Errorf("path = %q", got.Path)
	}
	data := bodyMap(t, got.Body)["data"].(map[string]any)
	p0 := data["attributes"].(map[string]any)["profiles"].(map[string]any)["data"].([]any)[0].(map[string]any)["attributes"].(map[string]any)
	if _, ok := p0["subscriptions"]; ok {
		t.Errorf("delete job must not carry subscriptions consent: %v", p0)
	}
}

func TestSuppressBuildsJob(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "suppress", "--email", "a@b.com")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Path != "/profile-suppression-bulk-create-jobs" {
		t.Errorf("path = %q", got.Path)
	}
	if bodyMap(t, got.Body)["data"].(map[string]any)["type"] != "profile-suppression-bulk-create-job" {
		t.Errorf("wrong job type: %s", got.Body)
	}
}

// --- lists ----------------------------------------------------------------

func TestListCreateFromName(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{"id":"L1"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "list", "create", "--name", "VIP")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	attrs := bodyMap(t, got.Body)["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["name"] != "VIP" {
		t.Errorf("name = %v", attrs["name"])
	}
}

func TestListAddProfilesRelationshipAndReceipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 204, ``, &got) // 204 No Content
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "list", "add-profiles", "L1", "--profile-id", "P1", "--profile-id", "P2")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/lists/L1/relationships/profiles" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	arr := bodyMap(t, got.Body)["data"].([]any)
	if len(arr) != 2 || arr[0].(map[string]any)["id"] != "P1" {
		t.Errorf("relationship data = %v", arr)
	}
	if !strings.Contains(stdout, `"status":"ok"`) {
		t.Errorf("204 must emit a receipt, got %q", stdout)
	}
}

func TestListRemoveProfilesUsesDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 204, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "list", "remove-profiles", "L1", "--profile-id", "P1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "DELETE" {
		t.Errorf("method = %s, want DELETE", got.Method)
	}
}

// --- segments -------------------------------------------------------------

func TestSegmentProfilesPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "segment", "profiles", "S1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Path != "/segments/S1/profiles" {
		t.Errorf("path = %q", got.Path)
	}
}

// --- campaigns ------------------------------------------------------------

func TestCampaignListDefaultsToEmailChannel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "campaign", "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Query.Get("filter") != "equals(messages.channel,'email')" {
		t.Errorf("filter = %q", got.Query.Get("filter"))
	}
}

func TestCampaignListChannelAndUserFilterCombine(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "campaign", "list", "--channel", "sms", "--filter", "equals(status,'Sent')")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	want := "and(equals(messages.channel,'sms'),equals(status,'Sent'))"
	if got.Query.Get("filter") != want {
		t.Errorf("filter = %q, want %q", got.Query.Get("filter"), want)
	}
}

func TestCampaignListRejectsBadChannel(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "campaign", "list", "--channel", "carrier-pigeon")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "channel") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCampaignSendBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{"data":{"id":"job1"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "campaign", "send", "--id", "C1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Path != "/campaign-send-jobs" {
		t.Errorf("path = %q", got.Path)
	}
	data := bodyMap(t, got.Body)["data"].(map[string]any)
	if data["type"] != "campaign-send-job" || data["id"] != "C1" {
		t.Errorf("send body = %v", data)
	}
}

func TestCampaignMessagesPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "messages", "C1")
	if code != 0 || got.Path != "/campaigns/C1/campaign-messages" {
		t.Errorf("path = %q code=%d", got.Path, code)
	}
}

// --- flows ----------------------------------------------------------------

func TestFlowStatusBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"F1"}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "flow", "status", "F1", "--status", "live")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "PATCH" || got.Path != "/flows/F1" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	attrs := bodyMap(t, got.Body)["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["status"] != "live" {
		t.Errorf("status = %v", attrs["status"])
	}
}

func TestFlowStatusRejectsBadValue(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "flow", "status", "F1", "--status", "paused")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "status") {
		t.Errorf("stderr = %q", stderr)
	}
}

// --- metrics --------------------------------------------------------------

func TestMetricAggregateRequiresData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "metric", "aggregate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--data") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestMetricAggregatePassesData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "metric", "aggregate",
		"--data", `{"data":{"type":"metric-aggregate","attributes":{"metric_id":"M1"}}}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/metric-aggregates" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	attrs := bodyMap(t, got.Body)["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["metric_id"] != "M1" {
		t.Errorf("body not passed through: %s", got.Body)
	}
}

// --- events ---------------------------------------------------------------

func TestEventCreateBuildsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "event", "create",
		"--metric", "Placed Order", "--email", "a@b.com",
		"--value", "9.99", "--properties", `{"order_id":"O1"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Path != "/events" {
		t.Errorf("path = %q", got.Path)
	}
	attrs := bodyMap(t, got.Body)["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["value"] != 9.99 {
		t.Errorf("value = %v, want 9.99", attrs["value"])
	}
	metric := attrs["metric"].(map[string]any)["data"].(map[string]any)["attributes"].(map[string]any)
	if metric["name"] != "Placed Order" {
		t.Errorf("metric name = %v", metric["name"])
	}
	if attrs["properties"].(map[string]any)["order_id"] != "O1" {
		t.Errorf("properties = %v", attrs["properties"])
	}
}

func TestEventCreateRejectsNonNumericValue(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 202, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "event", "create", "--metric", "M", "--email", "a@b.com", "--value", "lots")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--value") {
		t.Errorf("stderr = %q", stderr)
	}
}

// --- templates ------------------------------------------------------------

func TestTemplateGetPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"T1"}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "template", "get", "T1")
	if code != 0 || got.Path != "/templates/T1" {
		t.Errorf("path = %q code=%d", got.Path, code)
	}
}

// --- reports --------------------------------------------------------------

func TestReportCampaignValuesVsSeries(t *testing.T) {
	body := `{"data":{"type":"campaign-values-report","attributes":{"statistics":["opens"],"timeframe":{"key":"last_7_days"}}}}`

	var got capturedRequest
	srv := newServer(t, 200, `{"data":{}}`, &got)
	defer srv.Close()
	code, _, stderr := run(t, srv, "report", "campaign", "--data", body)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Path != "/campaign-values-reports" {
		t.Errorf("values path = %q", got.Path)
	}

	var got2 capturedRequest
	srv2 := newServer(t, 200, `{"data":{}}`, &got2)
	defer srv2.Close()
	code, _, _ = run(t, srv2, "report", "campaign", "--series", "--data", body)
	if code != 0 || got2.Path != "/campaign-series-reports" {
		t.Errorf("series path = %q code=%d", got2.Path, code)
	}
}

func TestReportFlowRequiresData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "report", "flow")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--data") {
		t.Errorf("stderr = %q", stderr)
	}
}

// --- errors ---------------------------------------------------------------

func TestAPIErrorExitsOneAndRendersMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 400, `{"errors":[{"code":"invalid","title":"Bad Request","detail":"missing field"}]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "list")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid") || !strings.Contains(stderr, "missing field") {
		t.Errorf("stderr = %q, want JSON:API error fields", stderr)
	}
}

func TestAPIErrorJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 404, `{"errors":[{"code":"not_found","title":"Not Found"}]}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "profile", "get", "P1", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, `"kind":"api"`) || !strings.Contains(stderr, `"status":404`) {
		t.Errorf("stderr = %q, want api envelope with status", stderr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"errors":[{"code":"not_authenticated","title":"Authentication Failed"}]}`, &got)
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{EnvAccessToken: "bad"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("401 must set CredentialRejected")
	}
}

func TestMissingTokenIsError(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "KLAVIYO_ACCESS_TOKEN") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "profile", "frobnicate")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}
