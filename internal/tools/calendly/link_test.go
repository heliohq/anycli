package calendly

import (
	"net/http"
	"testing"
)

func TestLinkCreateBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"resource":{"booking_url":"https://calendly.com/x"}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "link", "create", "--event-type", "ET-1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/scheduling_links" {
		t.Errorf("request = %s %s, want POST /scheduling_links", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["owner_type"] != "EventType" {
		t.Errorf("owner_type = %v, want EventType", body["owner_type"])
	}
	if body["owner"] != srv.URL+"/event_types/ET-1" {
		t.Errorf("owner = %v, want normalized event_type uri", body["owner"])
	}
	// max_event_count defaults to 1 (single-use).
	if body["max_event_count"].(float64) != 1 {
		t.Errorf("max_event_count = %v, want 1", body["max_event_count"])
	}
	if stdout == "" {
		t.Error("expected booking_url passthrough on stdout")
	}
}

func TestLinkCreateRequiresEventType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "link", "create")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for missing --event-type", code)
	}
}
