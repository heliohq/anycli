package forms

import (
	"net/http"
	"strings"
	"testing"
)

func TestRespondersList_PublishedViewOnly(t *testing.T) {
	// The editor row (no view) must be filtered out; only published-view rows
	// are responders.
	body := `{"permissions":[{"id":"owner","type":"user","role":"writer","emailAddress":"me@x.com"},{"id":"p1","type":"user","role":"reader","view":"published","emailAddress":"resp@x.com"},{"id":"p2","type":"anyone","role":"reader","view":"published"}]}`
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files/f1/permissions": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "responders", "list", "f1")
	if strings.Contains(stdout, "me@x.com") {
		t.Errorf("stdout = %q, editor (no view) must be excluded", stdout)
	}
	if !strings.Contains(stdout, "resp@x.com") || !strings.Contains(stdout, "anyone-with-link") {
		t.Errorf("stdout = %q, want the two published-view responders", stdout)
	}
	got := f.last(t, "GET", "/drive/v3/files/f1/permissions")
	if !strings.Contains(got.Query, "includePermissionsForView=published") {
		t.Errorf("query = %q, want includePermissionsForView=published", got.Query)
	}
}

func TestRespondersAdd_Anyone(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /drive/v3/files/f1/permissions": {http.StatusOK, `{"id":"p9","type":"anyone","role":"reader"}`},
	})
	stdout := f.runOK(t, "responders", "add", "f1", "--anyone")
	got := f.last(t, "POST", "/drive/v3/files/f1/permissions")
	body := string(got.Body)
	for _, want := range []string{`"view":"published"`, `"role":"reader"`, `"type":"anyone"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want %s", body, want)
		}
	}
	if !strings.Contains(stdout, "anyone with the link") {
		t.Errorf("stdout = %q, want the anyone-with-link confirmation", stdout)
	}
}

func TestRespondersAdd_MultipleEmailsSerial(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /drive/v3/files/f1/permissions": {http.StatusOK, `{"id":"p1","type":"user","role":"reader"}`},
	})
	stdout := f.runOK(t, "responders", "add", "f1", "--to", "a@b.c, d@e.f")
	// Two serial create calls, one per address.
	var creates int
	for _, r := range f.requests {
		if r.Method == "POST" && r.Path == "/drive/v3/files/f1/permissions" {
			creates++
		}
	}
	if creates != 2 {
		t.Errorf("create calls = %d, want 2 (one per address)", creates)
	}
	if !strings.Contains(stdout, "a@b.c") || !strings.Contains(stdout, "d@e.f") {
		t.Errorf("stdout = %q, want both addresses in the summary", stdout)
	}
}

func TestRespondersRemove_Anyone(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files/f1/permissions":         {http.StatusOK, `{"permissions":[{"id":"pAny","type":"anyone","role":"reader","view":"published"}]}`},
		"DELETE /drive/v3/files/f1/permissions/pAny": {http.StatusNoContent, ``},
	})
	stdout := f.runOK(t, "responders", "remove", "f1", "--anyone")
	f.last(t, "DELETE", "/drive/v3/files/f1/permissions/pAny")
	if !strings.Contains(stdout, "removed responder pAny") {
		t.Errorf("stdout = %q, want the removal confirmation", stdout)
	}
}

func TestRespondersRemove_NotFoundIsIdempotent(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /drive/v3/files/f1/permissions": {http.StatusOK, `{"permissions":[]}`},
	})
	stdout := f.runOK(t, "responders", "remove", "f1", "--to", "gone@x.com")
	if !strings.Contains(stdout, "already removed") {
		t.Errorf("stdout = %q, want the idempotent not-found message", stdout)
	}
	// No DELETE should have been attempted.
	for _, r := range f.requests {
		if r.Method == "DELETE" {
			t.Errorf("unexpected DELETE for a missing responder: %s", r.Path)
		}
	}
}
