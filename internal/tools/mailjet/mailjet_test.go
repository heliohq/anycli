package mailjet

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// wantBasicHeader is the exact Authorization header value for testBasic.
var wantBasicHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(testBasic))

func TestBasicAuthHeaderIsBase64OfUserinfo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"Count":0,"Data":[],"Total":0}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "contact", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Auth != wantBasicHeader {
		t.Errorf("Authorization = %q, want %q", got.Auth, wantBasicHeader)
	}
	// The header must be true Basic auth, not the raw userinfo string.
	if strings.Contains(got.Auth, ":") {
		t.Errorf("Authorization leaks raw userinfo: %q", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
}

func TestMissingCredentialFailsExit1(t *testing.T) {
	result, stdout, stderr := runNoCred(t, "contact", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, EnvBasicAuth) {
		t.Errorf("stderr = %q, want mention of %s", stderr, EnvBasicAuth)
	}
}

func TestContactListUnwrapsEnvelope(t *testing.T) {
	var got capturedRequest
	body := `{"Count":2,"Data":[{"ID":132,"Email":"a@example.com"},{"ID":145,"Email":"b@example.com"}],"Total":87}`
	srv := newServer(t, http.StatusOK, body, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "contact", "list", "--limit", "2", "--offset", "5")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/v3/REST/contact" {
		t.Errorf("path = %q, want /v3/REST/contact", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("Limit") != "2" || q.Get("Offset") != "5" {
		t.Errorf("query = %q, want Limit=2 Offset=5", got.Query)
	}
	out := decodeOut(t, stdout)
	if out["count"].(float64) != 2 || out["total"].(float64) != 87 {
		t.Errorf("out count/total = %v/%v, want 2/87", out["count"], out["total"])
	}
	data, ok := out["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("out data = %v, want 2-element array", out["data"])
	}
	// The raw {Count,Data,Total} wrapper must not leak into stdout.
	if strings.Contains(stdout, `"Data"`) || strings.Contains(stdout, `"Count"`) {
		t.Errorf("stdout leaks REST envelope: %s", stdout)
	}
}

func TestContactGetReturnsSingleObject(t *testing.T) {
	var got capturedRequest
	body := `{"Count":1,"Data":[{"ID":132,"Email":"a@example.com","Name":"A"}],"Total":1}`
	srv := newServer(t, http.StatusOK, body, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "contact", "get", "--id", "a@example.com")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/v3/REST/contact/a@example.com" {
		t.Errorf("path = %q, want /v3/REST/contact/a@example.com", got.Path)
	}
	out := decodeOut(t, stdout)
	if out["Email"] != "a@example.com" {
		t.Errorf("out = %v, want single contact object", out)
	}
	if _, isList := out["data"]; isList {
		t.Errorf("get should emit the object, not a list wrapper: %s", stdout)
	}
}

func TestContactCreatePostsBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"Count":1,"Data":[{"ID":999,"Email":"new@example.com"}],"Total":1}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "contact", "create", "--email", "new@example.com", "--name", "New", "--excluded")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v3/REST/contact" {
		t.Errorf("%s %s, want POST /v3/REST/contact", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["Email"] != "new@example.com" || b["Name"] != "New" || b["IsExcludedFromCampaigns"] != true {
		t.Errorf("body = %v, want Email/Name/IsExcludedFromCampaigns set", b)
	}
}

func TestSendBuildsV31Message(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"Messages":[{"Status":"success","To":[{"Email":"p@example.com","MessageID":456,"MessageUUID":"abc"}]}]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv,
		"send",
		"--from-email", "pilot@example.com", "--from-name", "Pilot",
		"--to", "Passenger <p@example.com>",
		"--to", "q@example.com",
		"--subject", "Flight plan",
		"--text", "hello",
		"--html", "<b>hello</b>",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v3.1/send" {
		t.Errorf("%s %s, want POST /v3.1/send", got.Method, got.Path)
	}
	b := decodeBody(t, got.Body)
	msgs, ok := b["Messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("Messages = %v, want 1-element array", b["Messages"])
	}
	m := msgs[0].(map[string]any)
	from := m["From"].(map[string]any)
	if from["Email"] != "pilot@example.com" || from["Name"] != "Pilot" {
		t.Errorf("From = %v", from)
	}
	to := m["To"].([]any)
	if len(to) != 2 {
		t.Fatalf("To len = %d, want 2", len(to))
	}
	first := to[0].(map[string]any)
	if first["Email"] != "p@example.com" || first["Name"] != "Passenger" {
		t.Errorf("To[0] = %v, want parsed Name <email>", first)
	}
	second := to[1].(map[string]any)
	if second["Email"] != "q@example.com" {
		t.Errorf("To[1] = %v, want bare email", second)
	}
	if m["Subject"] != "Flight plan" || m["TextPart"] != "hello" || m["HTMLPart"] != "<b>hello</b>" {
		t.Errorf("message content = %v", m)
	}
	// The v3.1 send response is passed through verbatim.
	if !strings.Contains(stdout, `"Status":"success"`) {
		t.Errorf("stdout = %s, want verbatim send response", stdout)
	}
}

func TestSendWithTemplateSetsTemplateLanguage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"Messages":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"send", "--from-email", "pilot@example.com",
		"--to", "p@example.com",
		"--template-id", "12345",
		"--variables-json", `{"name":"Ada"}`,
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	m := decodeBody(t, got.Body)["Messages"].([]any)[0].(map[string]any)
	if m["TemplateID"].(float64) != 12345 || m["TemplateLanguage"] != true {
		t.Errorf("template fields = %v", m)
	}
	vars, ok := m["Variables"].(map[string]any)
	if !ok || vars["name"] != "Ada" {
		t.Errorf("Variables = %v", m["Variables"])
	}
}

func TestSendRequiresBodyOrTemplate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "send", "--from-email", "pilot@example.com", "--to", "p@example.com")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", exit)
	}
	if stdout != "" || stderr == "" {
		t.Errorf("stdout=%q stderr=%q, want empty stdout + usage error", stdout, stderr)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call should have been made, saw %s %s", got.Method, got.Path)
	}
}

func TestListAddContactPostsListRecipient(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"Count":1,"Data":[{"ID":1}],"Total":1}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "list", "add-contact", "--contact-id", "132", "--list-id", "77", "--unsubscribed")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/v3/REST/listrecipient" {
		t.Errorf("path = %q, want /v3/REST/listrecipient", got.Path)
	}
	b := decodeBody(t, got.Body)
	if b["ContactID"].(float64) != 132 || b["ListID"].(float64) != 77 || b["IsUnsubscribed"] != true {
		t.Errorf("body = %v", b)
	}
}

func TestTemplateGetHitsDetailContent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"Count":1,"Data":[{"Html-part":"<p>hi</p>"}],"Total":1}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "template", "get", "--id", "555")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/v3/REST/template/555/detailcontent" {
		t.Errorf("path = %q, want /v3/REST/template/555/detailcontent", got.Path)
	}
}

func TestMessageListAppliesFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"Count":0,"Data":[],"Total":0}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "message", "list", "--campaign-id", "42", "--contact-id", "7")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	q := parseQuery(t, got.Query)
	if q.Get("Campaign") != "42" || q.Get("Contact") != "7" {
		t.Errorf("query = %q, want Campaign=42 Contact=7", got.Query)
	}
}

func TestStatCountersDefaultsAndQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"Count":1,"Data":[{"MessageClickedCount":3}],"Total":1}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "stat", "counters", "--counter-source", "Campaign", "--source-id", "42")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/v3/REST/statcounters" {
		t.Errorf("path = %q, want /v3/REST/statcounters", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("CounterSource") != "Campaign" || q.Get("SourceID") != "42" {
		t.Errorf("query = %q", got.Query)
	}
	if q.Get("CounterTiming") != "Message" || q.Get("CounterResolution") != "Lifetime" {
		t.Errorf("defaults not applied: %q", got.Query)
	}
}

func TestStatRecipientESPRequiresCampaign(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"Count":0,"Data":[],"Total":0}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "stat", "recipient-esp", "--campaign-id", "42")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Path != "/v3/REST/statistics/recipient-esp" {
		t.Errorf("path = %q, want /v3/REST/statistics/recipient-esp", got.Path)
	}
	if parseQuery(t, got.Query).Get("CampaignId") != "42" {
		t.Errorf("query = %q, want CampaignId=42", got.Query)
	}
}

func TestAPIErrorExit1AndJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"ErrorMessage":"bad input","ErrorInfo":"detail"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "contact", "list", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not JSON error envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("error envelope = %+v, want kind=api status=400", env.Error)
	}
	if !strings.Contains(env.Error.Message, "bad input") {
		t.Errorf("message = %q, want provider error text", env.Error.Message)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"ErrorMessage":"unauthorized"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "contact", "list")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 + credential rejected", result)
	}
}

func TestRegionUSSwitchesHost(t *testing.T) {
	// The --region us flag must resolve to the US host; BaseURL field is ignored
	// when the flag selects a concrete region. We assert on resolveBaseURL
	// directly because the httptest host cannot be the real US host.
	svc := &Service{BaseURL: "https://ignored.example"}
	root := svc.newRoot(testBasic)
	root.SetArgs([]string{"contact", "list", "--region", "us"})
	// Walk to the leaf command to read the resolved flags.
	cmd, _, err := root.Find([]string{"contact", "list"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if err := cmd.ParseFlags([]string{"--region", "us"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	base, err := svc.resolveBaseURL(cmd)
	if err != nil {
		t.Fatalf("resolveBaseURL: %v", err)
	}
	if base != usBaseURL {
		t.Errorf("base = %q, want %q", base, usBaseURL)
	}
}

func TestBaseURLFlagOverridesRegion(t *testing.T) {
	svc := &Service{}
	root := svc.newRoot(testBasic)
	cmd, _, err := root.Find([]string{"contact", "list"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if err := cmd.ParseFlags([]string{"--base-url", "https://custom.example/", "--region", "us"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	base, err := svc.resolveBaseURL(cmd)
	if err != nil {
		t.Fatalf("resolveBaseURL: %v", err)
	}
	if base != "https://custom.example" {
		t.Errorf("base = %q, want https://custom.example (trailing slash trimmed)", base)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "contact", "bogus")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for unknown subcommand", exit)
	}
}
