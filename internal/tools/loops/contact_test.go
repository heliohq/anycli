package loops

import (
	"net/http"
	"strings"
	"testing"
)

func TestContactCreate_FirstClassAndCustomFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"id":"c1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create",
		"--email", "a@e.com", "--first-name", "Ada", "--last-name", "Lovelace",
		"--user-group", "beta", "--subscribed=false",
		"--mailing-list", "list1=true", "--mailing-list", "list2=false",
		"--property", "plan=pro", "--property", "seats=5", "--property", "trial=true",
		"--properties-json", `{"region":"eu"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/contacts/create" {
		t.Errorf("request = %s %s, want POST /v1/contacts/create", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@e.com" || body["firstName"] != "Ada" || body["lastName"] != "Lovelace" {
		t.Errorf("first-class fields = %v", body)
	}
	if body["userGroup"] != "beta" {
		t.Errorf("userGroup = %v, want beta", body["userGroup"])
	}
	if body["subscribed"] != false {
		t.Errorf("subscribed = %v, want false", body["subscribed"])
	}
	// typed-coerced custom properties
	if body["plan"] != "pro" {
		t.Errorf("plan = %v, want string pro", body["plan"])
	}
	if body["seats"] != float64(5) {
		t.Errorf("seats = %v (%T), want number 5", body["seats"], body["seats"])
	}
	if body["trial"] != true {
		t.Errorf("trial = %v, want bool true", body["trial"])
	}
	if body["region"] != "eu" {
		t.Errorf("region (from --properties-json) = %v, want eu", body["region"])
	}
	mailing, ok := body["mailingLists"].(map[string]any)
	if !ok || mailing["list1"] != true || mailing["list2"] != false {
		t.Errorf("mailingLists = %v, want {list1:true,list2:false}", body["mailingLists"])
	}
}

// TestContactCreate_SubscribedOmittedWhenUnset proves subscribed is only sent
// when the flag is explicitly set.
func TestContactCreate_SubscribedOmittedWhenUnset(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "create", "--email", "a@e.com")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["subscribed"]; ok {
		t.Errorf("subscribed should be omitted when unset, body = %v", body)
	}
}

func TestContactCreate_RequiresEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "contact", "create", "--first-name", "Ada")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (missing required --email)", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call, saw %s", got.Method)
	}
	if !strings.Contains(strings.ToLower(stderr), "email") {
		t.Errorf("stderr = %q, want an email-required message", stderr)
	}
}

func TestContactUpdate_UpsertByUserID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "update", "--user-id", "u1", "--first-name", "Grace")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPut || got.Path != "/v1/contacts/update" {
		t.Errorf("request = %s %s, want PUT /v1/contacts/update", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["userId"] != "u1" || body["firstName"] != "Grace" {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["email"]; ok {
		t.Errorf("email should be omitted, body = %v", body)
	}
}

func TestContactUpdate_RequiresAnIdentifier(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "update", "--first-name", "Grace")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (needs email or user-id)", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call, saw %s", got.Method)
	}
}

func TestContactFind_ByEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":"c1"}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "find", "--email", "a@e.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/contacts/find" {
		t.Errorf("request = %s %s, want GET /v1/contacts/find", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("email") != "a@e.com" {
		t.Errorf("query = %q, want email=a@e.com", got.Query)
	}
}

func TestContactFind_ExactlyOneIdentifier(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	// both provided → usage error, no call
	code, _, _ := run(t, srv, "contact", "find", "--email", "a@e.com", "--user-id", "u1")
	if code != 2 {
		t.Fatalf("both identifiers: exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call when both identifiers set, saw %s", got.Method)
	}

	// neither provided → usage error
	got = capturedRequest{}
	code, _, _ = run(t, srv, "contact", "find")
	if code != 2 {
		t.Fatalf("no identifier: exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call when no identifier set, saw %s", got.Method)
	}
}

// TestContactDelete_RejectsBothForwardsOne is the load-bearing provider-quirk
// test: the OpenAPI schema marks both email and userId required, but the live
// API rejects the both-provided case. The CLI enforces exactly one client-side
// and forwards only that identifier.
func TestContactDelete_RejectsBothForwardsOne(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true}`, &got)
	defer srv.Close()

	// both → exit 2, no call
	code, _, _ := run(t, srv, "contact", "delete", "--email", "a@e.com", "--user-id", "u1")
	if code != 2 {
		t.Fatalf("both identifiers: exit code = %d, want 2", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call when both identifiers set, saw %s", got.Method)
	}

	// exactly one → forwards only that identifier
	got = capturedRequest{}
	code, _, _ = run(t, srv, "contact", "delete", "--user-id", "u1")
	if code != 0 {
		t.Fatalf("single identifier: exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/contacts/delete" {
		t.Errorf("request = %s %s, want POST /v1/contacts/delete", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["userId"] != "u1" {
		t.Errorf("userId = %v, want u1", body["userId"])
	}
	if _, ok := body["email"]; ok {
		t.Errorf("email must not be forwarded, body = %v", body)
	}
}

func TestContactSuppression_GetAndRemove(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"suppressed":true}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "suppression", "get", "--email", "a@e.com")
	if code != 0 {
		t.Fatalf("get exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/contacts/suppression" {
		t.Errorf("get request = %s %s, want GET /v1/contacts/suppression", got.Method, got.Path)
	}

	got = capturedRequest{}
	code, _, _ = run(t, srv, "contact", "suppression", "remove", "--user-id", "u1")
	if code != 0 {
		t.Fatalf("remove exit code = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/v1/contacts/suppression" {
		t.Errorf("remove request = %s %s, want DELETE /v1/contacts/suppression", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("userId") != "u1" {
		t.Errorf("query = %q, want userId=u1", got.Query)
	}
}

func TestContactProperty_ListAndCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"key":"plan"}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact-property", "list", "--list", "custom")
	if code != 0 {
		t.Fatalf("list exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/contacts/properties" {
		t.Errorf("list request = %s %s", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("list") != "custom" {
		t.Errorf("query = %q, want list=custom", got.Query)
	}

	got = capturedRequest{}
	code, _, _ = run(t, srv, "contact-property", "create", "--name", "planName", "--type", "string")
	if code != 0 {
		t.Fatalf("create exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/contacts/properties" {
		t.Errorf("create request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "planName" || body["type"] != "string" {
		t.Errorf("body = %v, want name=planName type=string", body)
	}
}

// TestContactProperty_CreateRequiresNameAndType guards the required flags.
func TestContactProperty_CreateRequiresNameAndType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact-property", "create", "--name", "planName")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (missing --type)", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call, saw %s", got.Method)
	}
}
