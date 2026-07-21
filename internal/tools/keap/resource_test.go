package keap

import (
	"reflect"
	"strings"
	"testing"
)

func TestContactListPassesQueryParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/contacts": {status: 200, body: `{"contacts":[]}`},
	})
	defer srv.Close()

	res := run(t, srv, "tok", "contact", "list",
		"--page-size", "25", "--page-token", "abc",
		"--filter", "email==a@b.com", "--order-by", "given_name", "--fields", "id,email_addresses")
	if res.exitCode != 0 {
		t.Fatalf("exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	req := findReq(reqs, "GET", "/v2/contacts")
	if req == nil {
		t.Fatal("no GET /v2/contacts")
	}
	want := map[string]string{
		"page_size": "25", "page_token": "abc",
		"filter": "email==a@b.com", "order_by": "given_name", "fields": "id,email_addresses",
	}
	for k, v := range want {
		if got := req.Query.Get(k); got != v {
			t.Errorf("query[%s] = %q, want %q", k, got, v)
		}
	}
}

func TestContactCreateBodyShape(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/contacts": {status: 201, body: `{"id":"7"}`},
	})
	defer srv.Close()

	res := run(t, srv, "tok", "contact", "create",
		"--email", "jo@x.com", "--given-name", "Jo", "--family-name", "Ng", "--phone", "+15551234567")
	if res.exitCode != 0 {
		t.Fatalf("exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	req := findReq(reqs, "POST", "/v2/contacts")
	if req == nil {
		t.Fatal("no POST /v2/contacts")
	}
	if req.ContentType != "application/json" {
		t.Errorf("content-type = %q", req.ContentType)
	}
	body := bodyMap(t, req.Body)
	if body["given_name"] != "Jo" || body["family_name"] != "Ng" {
		t.Errorf("names wrong: %+v", body)
	}
	emails, ok := body["email_addresses"].([]any)
	if !ok || len(emails) != 1 {
		t.Fatalf("email_addresses shape wrong: %+v", body["email_addresses"])
	}
	e := emails[0].(map[string]any)
	if e["email"] != "jo@x.com" || e["field"] != "EMAIL1" {
		t.Errorf("email object wrong: %+v", e)
	}
	phones := body["phone_numbers"].([]any)
	p := phones[0].(map[string]any)
	if p["number"] != "+15551234567" || p["field"] != "PHONE1" {
		t.Errorf("phone object wrong: %+v", p)
	}
}

func TestContactCreateJSONBodyOverlaysConvenienceFlags(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/contacts": {status: 201, body: `{"id":"7"}`},
	})
	defer srv.Close()

	res := run(t, srv, "tok", "contact", "create",
		"--given-name", "Jo",
		"--json-body", `{"custom_fields":[{"id":9,"content":"x"}],"given_name":"Override"}`)
	if res.exitCode != 0 {
		t.Fatalf("exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	body := bodyMap(t, findReq(reqs, "POST", "/v2/contacts").Body)
	if body["given_name"] != "Override" {
		t.Errorf("json-body should win over convenience flag; got %v", body["given_name"])
	}
	if _, ok := body["custom_fields"]; !ok {
		t.Errorf("custom_fields not merged: %+v", body)
	}
}

func TestContactCreateNoFieldsIsUsageError(t *testing.T) {
	res := run(t, nil, "tok", "contact", "create")
	if res.exitCode != 2 {
		t.Fatalf("exit %d, want 2 for empty create body", res.exitCode)
	}
}

func TestContactUpdateAndDeleteMethods(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /v2/contacts/42":  {status: 200, body: `{"id":"42"}`},
		"DELETE /v2/contacts/42": {status: 204, body: ``},
	})
	defer srv.Close()

	if res := run(t, srv, "tok", "contact", "update", "42", "--job-title", "CTO"); res.exitCode != 0 {
		t.Fatalf("update exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	if body := bodyMap(t, findReq(reqs, "PATCH", "/v2/contacts/42").Body); body["job_title"] != "CTO" {
		t.Errorf("update body wrong: %+v", body)
	}
	if res := run(t, srv, "tok", "contact", "delete", "42"); res.exitCode != 0 {
		t.Fatalf("delete exit %d; stderr=%s", res.exitCode, res.stderr)
	}
}

func TestTagApplyAndRemoveBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/tags/5/contacts:applyTags":  {status: 200, body: `{}`},
		"POST /v2/tags/5/contacts:removeTags": {status: 200, body: `{}`},
	})
	defer srv.Close()

	for _, tc := range []struct{ verb, path string }{
		{"apply", "/v2/tags/5/contacts:applyTags"},
		{"remove", "/v2/tags/5/contacts:removeTags"},
	} {
		res := run(t, srv, "tok", "tag", tc.verb, "5", "--contact-id", "1", "--contact-id", "2")
		if res.exitCode != 0 {
			t.Fatalf("%s exit %d; stderr=%s", tc.verb, res.exitCode, res.stderr)
		}
		req := findReq(reqs, "POST", tc.path)
		if req == nil {
			t.Fatalf("no POST %s", tc.path)
		}
		body := bodyMap(t, req.Body)
		ids, ok := body["contact_ids"].([]any)
		if !ok || !reflect.DeepEqual([]any{"1", "2"}, ids) {
			t.Errorf("%s contact_ids = %+v, want [1 2]", tc.verb, body["contact_ids"])
		}
	}
}

func TestTagApplyRequiresContactID(t *testing.T) {
	res := run(t, nil, "tok", "tag", "apply", "5")
	if res.exitCode != 2 {
		t.Fatalf("exit %d, want 2 when no --contact-id", res.exitCode)
	}
}

func TestOpportunityCreateRequiresTitleAndStagesPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/opportunities":       {status: 201, body: `{"id":"1"}`},
		"GET /v2/opportunities/stages": {status: 200, body: `{"stages":[]}`},
	})
	defer srv.Close()

	if res := run(t, nil, "tok", "opportunity", "create", "--contact-id", "3"); res.exitCode != 2 {
		t.Fatalf("missing --title should be usage error, got exit %d", res.exitCode)
	}
	res := run(t, srv, "tok", "opportunity", "create", "--title", "Big Deal", "--stage-id", "9")
	if res.exitCode != 0 {
		t.Fatalf("create exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	body := bodyMap(t, findReq(reqs, "POST", "/v2/opportunities").Body)
	if body["opportunity_title"] != "Big Deal" || body["stage_id"] != "9" {
		t.Errorf("opportunity body wrong: %+v", body)
	}
	if res := run(t, srv, "tok", "opportunity", "stages"); res.exitCode != 0 {
		t.Fatalf("stages exit %d; stderr=%s", res.exitCode, res.stderr)
	}
}

func TestTaskCreateRequiresAssignedUser(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/tasks": {status: 201, body: `{"id":"1"}`},
	})
	defer srv.Close()

	if res := run(t, nil, "tok", "task", "create", "--title", "Follow up"); res.exitCode != 2 {
		t.Fatalf("missing --assigned-to-user-id should be usage error, got exit %d", res.exitCode)
	}
	res := run(t, srv, "tok", "task", "create", "--assigned-to-user-id", "77", "--title", "Follow up", "--contact-id", "3")
	if res.exitCode != 0 {
		t.Fatalf("create exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	body := bodyMap(t, findReq(reqs, "POST", "/v2/tasks").Body)
	if body["assigned_to_user_id"] != "77" || body["title"] != "Follow up" {
		t.Errorf("task body wrong: %+v", body)
	}
}

func TestNoteContactScopedPaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/contacts/3/notes":       {status: 200, body: `{"notes":[]}`},
		"POST /v2/contacts/3/notes":      {status: 201, body: `{"id":"n1"}`},
		"GET /v2/contacts/3/notes/n1":    {status: 200, body: `{"id":"n1"}`},
		"DELETE /v2/contacts/3/notes/n1": {status: 204, body: ``},
	})
	defer srv.Close()

	if res := run(t, srv, "tok", "note", "list", "--contact-id", "3"); res.exitCode != 0 {
		t.Fatalf("list exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	res := run(t, srv, "tok", "note", "create", "--contact-id", "3", "--user-id", "77", "--text", "hi")
	if res.exitCode != 0 {
		t.Fatalf("create exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	body := bodyMap(t, findReq(reqs, "POST", "/v2/contacts/3/notes").Body)
	if body["user_id"] != "77" || body["text"] != "hi" {
		t.Errorf("note body wrong: %+v", body)
	}
	if res := run(t, srv, "tok", "note", "get", "--contact-id", "3", "--note-id", "n1"); res.exitCode != 0 {
		t.Fatalf("get exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	if res := run(t, srv, "tok", "note", "delete", "--contact-id", "3", "--note-id", "n1"); res.exitCode != 0 {
		t.Fatalf("delete exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	if res := run(t, nil, "tok", "note", "create", "--contact-id", "3", "--text", "hi"); res.exitCode != 2 {
		t.Fatalf("missing --user-id should be usage error, got %d", res.exitCode)
	}
}

func TestEmailSendBodyAndRequired(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/emails:send": {status: 200, body: `{}`},
	})
	defer srv.Close()

	if res := run(t, nil, "tok", "email", "send", "--subject", "Hi", "--user-id", "1"); res.exitCode != 2 {
		t.Fatalf("missing --contact should be usage error, got %d", res.exitCode)
	}
	res := run(t, srv, "tok", "email", "send",
		"--contact", "1", "--contact", "2", "--subject", "Hi", "--user-id", "9", "--html", "<b>x</b>")
	if res.exitCode != 0 {
		t.Fatalf("send exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	body := bodyMap(t, findReq(reqs, "POST", "/v2/emails:send").Body)
	ids, _ := body["contacts"].([]any)
	if !reflect.DeepEqual([]any{"1", "2"}, ids) {
		t.Errorf("contacts = %+v, want [1 2]", body["contacts"])
	}
	if body["subject"] != "Hi" || body["user_id"] != "9" || body["html_content"] != "<b>x</b>" {
		t.Errorf("email body wrong: %+v", body)
	}
}

func TestAutomationAddContactsPathAndBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/automations/10/sequences/20:addContacts": {status: 200, body: `{}`},
	})
	defer srv.Close()

	if res := run(t, nil, "tok", "automation", "add-contacts", "10", "--contact-id", "1"); res.exitCode != 2 {
		t.Fatalf("missing --sequence-id should be usage error, got %d", res.exitCode)
	}
	res := run(t, srv, "tok", "automation", "add-contacts", "10", "--sequence-id", "20", "--contact-id", "1")
	if res.exitCode != 0 {
		t.Fatalf("add-contacts exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	req := findReq(reqs, "POST", "/v2/automations/10/sequences/20:addContacts")
	if req == nil {
		t.Fatal("no addContacts request")
	}
	body := bodyMap(t, req.Body)
	if ids, _ := body["contact_ids"].([]any); !reflect.DeepEqual([]any{"1"}, ids) {
		t.Errorf("contact_ids = %+v", body["contact_ids"])
	}
}

func TestUserMeHitsUserinfo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/oauth/connect/userinfo": {status: 200, body: `{"sub":"abc"}`},
		"GET /v2/users":                  {status: 200, body: `{"users":[]}`},
	})
	defer srv.Close()

	if res := run(t, srv, "tok", "user", "me"); res.exitCode != 0 {
		t.Fatalf("me exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	if findReq(reqs, "GET", "/v2/oauth/connect/userinfo") == nil {
		t.Error("user me did not hit userinfo")
	}
	if res := run(t, srv, "tok", "user", "list"); res.exitCode != 0 {
		t.Fatalf("list exit %d; stderr=%s", res.exitCode, res.stderr)
	}
}

func TestCampaignReadPaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/campaigns":   {status: 200, body: `{"campaigns":[]}`},
		"GET /v2/campaigns/7": {status: 200, body: `{"id":"7"}`},
	})
	defer srv.Close()

	if res := run(t, srv, "tok", "campaign", "list"); res.exitCode != 0 {
		t.Fatalf("list exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	if res := run(t, srv, "tok", "campaign", "get", "7"); res.exitCode != 0 {
		t.Fatalf("get exit %d; stderr=%s", res.exitCode, res.stderr)
	}
}

func TestCompanyCreateRequiresName(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/companies": {status: 201, body: `{"id":"1"}`},
	})
	defer srv.Close()

	if res := run(t, nil, "tok", "company", "create", "--website", "x.com"); res.exitCode != 2 {
		t.Fatalf("missing --company-name should be usage error, got %d", res.exitCode)
	}
	res := run(t, srv, "tok", "company", "create", "--company-name", "Acme")
	if res.exitCode != 0 {
		t.Fatalf("create exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	if body := bodyMap(t, findReq(reqs, "POST", "/v2/companies").Body); body["company_name"] != "Acme" {
		t.Errorf("company body wrong: %+v", body)
	}
}

func TestErrorMessageSurfacedPlain(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/contacts/9": {status: 404, body: `{"message":"Contact not found"}`},
	})
	defer srv.Close()

	res := run(t, srv, "tok", "contact", "get", "9")
	if res.exitCode != 1 {
		t.Fatalf("exit %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "Contact not found") {
		t.Errorf("stderr = %q, want provider message", res.stderr)
	}
	if res.rejected {
		t.Error("404 should not reject the credential")
	}
}
