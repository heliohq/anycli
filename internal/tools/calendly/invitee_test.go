package calendly

import (
	"net/http"
	"testing"
)

func TestInviteeNoShowMarksWithFullURI(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"resource":{}}`, &got)
	defer srv.Close()

	inviteeURI := "https://api.calendly.com/scheduled_events/EV/invitees/INV"
	code, _, _ := run(t, srv, "invitee", "no-show", inviteeURI)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/invitee_no_shows" {
		t.Errorf("request = %s %s, want POST /invitee_no_shows", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["invitee"] != inviteeURI {
		t.Errorf("invitee = %v, want %q", body["invitee"], inviteeURI)
	}
}

func TestInviteeNoShowRejectsBareUUID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "invitee", "no-show", "INV-123")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for ambiguous bare UUID", code)
	}
	if got.Method != "" {
		t.Errorf("no request should be sent for a rejected bare UUID, got %s %s", got.Method, got.Path)
	}
	_ = stderr
}

func TestInviteeNoShowUndoDeletes(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "invitee", "no-show", "https://api.calendly.com/invitee_no_shows/NS-9", "--undo")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/invitee_no_shows/NS-9" {
		t.Errorf("request = %s %s, want DELETE /invitee_no_shows/NS-9", got.Method, got.Path)
	}
}
