package calendly

import (
	"net/http"
	"strings"
	"testing"
)

func TestBookCreateBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"resource":{}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv,
		"book", "create",
		"--event-type", "ET-1",
		"--start", "2026-08-01T15:00:00Z",
		"--name", "Ada",
		"--email", "ada@example.com",
		"--timezone", "America/New_York",
		"--location-kind", "phone_call",
		"--location", "+15551234",
		"--guest", "g1@example.com",
		"--guest", "g2@example.com",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/invitees" {
		t.Errorf("request = %s %s, want POST /invitees", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["event_type"] != srv.URL+"/event_types/ET-1" {
		t.Errorf("event_type = %v, want normalized uri", body["event_type"])
	}
	if body["start_time"] != "2026-08-01T15:00:00Z" {
		t.Errorf("start_time = %v", body["start_time"])
	}
	invitee, ok := body["invitee"].(map[string]any)
	if !ok || invitee["name"] != "Ada" || invitee["email"] != "ada@example.com" || invitee["timezone"] != "America/New_York" {
		t.Errorf("invitee = %v, want {name,email,timezone}", body["invitee"])
	}
	loc, ok := body["location"].(map[string]any)
	if !ok || loc["kind"] != "phone_call" || loc["location"] != "+15551234" {
		t.Errorf("location = %v, want {kind,location}", body["location"])
	}
	guests, ok := body["guests"].([]any)
	if !ok || len(guests) != 2 || guests[0] != "g1@example.com" {
		t.Errorf("guests = %v, want two emails", body["guests"])
	}
}

func TestBookCreateSurfaces403Verbatim(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"title":"Permission Denied","message":"This feature requires a paid plan"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv,
		"book", "create",
		"--event-type", "ET-1",
		"--start", "2026-08-01T15:00:00Z",
		"--name", "Ada",
		"--email", "ada@example.com",
		"--timezone", "UTC",
	)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for API 403", code)
	}
	if !strings.Contains(stderr, "paid plan") {
		t.Errorf("stderr = %q, want the provider 403 surfaced verbatim", stderr)
	}
}

func TestBookCreateRequiresCoreFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "book", "create", "--event-type", "ET-1")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for missing required flags", code)
	}
}
