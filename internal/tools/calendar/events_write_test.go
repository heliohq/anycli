package calendar

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestEventsCreate_MeetAttendeesAndSendUpdates(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /calendar/v3/calendars/primary/events": {http.StatusOK, `{"id":"new1","summary":"Sync","start":{"dateTime":"2026-07-16T10:00:00-07:00"},"hangoutLink":"https://meet.google.com/abc-defg-hij"}`},
	})
	stdout := f.runOK(t, "events", "create",
		"--summary", "Sync",
		"--from", "2026-07-16T10:00:00-07:00", "--to", "2026-07-16T11:00:00-07:00",
		"--attendee", "alice@example.com", "--attendee", "bob@example.com",
		"--description", "Weekly sync", "--location", "Room 1",
		"--recurrence", "RRULE:FREQ=WEEKLY;COUNT=4",
		"--reminder", "10", "--reminder", "30",
		"--meet")
	got := f.last(t, "POST", "/calendar/v3/calendars/primary/events")

	// default --send-updates is all; --meet must set conferenceDataVersion=1.
	if !strings.Contains(got.Query, "sendUpdates=all") {
		t.Errorf("query = %q, want sendUpdates=all default", got.Query)
	}
	if !strings.Contains(got.Query, "conferenceDataVersion=1") {
		t.Errorf("query = %q, want conferenceDataVersion=1 for --meet", got.Query)
	}

	var payload struct {
		Summary    string                    `json:"summary"`
		Start      struct{ DateTime string } `json:"start"`
		Attendees  []struct{ Email string }  `json:"attendees"`
		Recurrence []string                  `json:"recurrence"`
		Reminders  struct {
			UseDefault bool `json:"useDefault"`
			Overrides  []struct {
				Method  string `json:"method"`
				Minutes int    `json:"minutes"`
			} `json:"overrides"`
		} `json:"reminders"`
		ConferenceData struct {
			CreateRequest struct {
				RequestID             string                `json:"requestId"`
				ConferenceSolutionKey struct{ Type string } `json:"conferenceSolutionKey"`
			} `json:"createRequest"`
		} `json:"conferenceData"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload.Summary != "Sync" || payload.Start.DateTime != "2026-07-16T10:00:00-07:00" {
		t.Errorf("payload summary/start = %q/%q", payload.Summary, payload.Start.DateTime)
	}
	if len(payload.Attendees) != 2 || payload.Attendees[0].Email != "alice@example.com" {
		t.Errorf("attendees = %+v, want alice + bob", payload.Attendees)
	}
	if len(payload.Recurrence) != 1 || payload.Recurrence[0] != "RRULE:FREQ=WEEKLY;COUNT=4" {
		t.Errorf("recurrence = %v, want the verbatim RRULE", payload.Recurrence)
	}
	if payload.Reminders.UseDefault || len(payload.Reminders.Overrides) != 2 || payload.Reminders.Overrides[0].Minutes != 10 || payload.Reminders.Overrides[0].Method != "popup" {
		t.Errorf("reminders = %+v, want two popup overrides", payload.Reminders)
	}
	if payload.ConferenceData.CreateRequest.RequestID == "" || payload.ConferenceData.CreateRequest.ConferenceSolutionKey.Type != "hangoutsMeet" {
		t.Errorf("conferenceData = %+v, want a requestId + hangoutsMeet", payload.ConferenceData)
	}
	if !strings.Contains(stdout, "meet: https://meet.google.com/abc-defg-hij") {
		t.Errorf("human output = %q, want the Meet link", stdout)
	}
}

func TestEventsCreate_MeetRequestIDIsUnique(t *testing.T) {
	// With the real generator (not the fixture's fixed id), two creates must
	// produce distinct conference requestIds so a conference is never reused.
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		s := &Service{}
		id := s.requestID()
		if id == "" {
			t.Fatal("requestID returned empty")
		}
		if seen[id] {
			t.Fatalf("requestID collision on %q", id)
		}
		seen[id] = true
	}
}

func TestEventsCreate_AllDayUsesDate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /calendar/v3/calendars/primary/events": {http.StatusOK, `{"id":"a1"}`},
	})
	f.runOK(t, "events", "create", "--summary", "PTO", "--all-day", "--from", "2026-07-20", "--to", "2026-07-21")
	got := f.last(t, "POST", "/calendar/v3/calendars/primary/events")
	var payload struct {
		Start map[string]string `json:"start"`
		End   map[string]string `json:"end"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload.Start["date"] != "2026-07-20" || payload.Start["dateTime"] != "" {
		t.Errorf("start = %v, want the date field for all-day", payload.Start)
	}
}

func TestEventsUpdate_UsesPatchPartial(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /calendar/v3/calendars/primary/events/e1": {http.StatusOK, `{"id":"e1","summary":"Renamed"}`},
	})
	f.runOK(t, "events", "update", "e1", "--summary", "Renamed")
	got := f.last(t, "PATCH", "/calendar/v3/calendars/primary/events/e1")
	// Only the changed field must be in the body — never a full replace that
	// would silently clear attendees/recurrence.
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if _, ok := payload["summary"]; !ok {
		t.Errorf("payload = %v, want the summary field", payload)
	}
	for _, absent := range []string{"start", "end", "attendees", "recurrence", "description", "location", "reminders"} {
		if _, ok := payload[absent]; ok {
			t.Errorf("payload contains %q, but patch must only send changed fields", absent)
		}
	}
}

func TestEventsDelete_SendUpdatesQuery(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /calendar/v3/calendars/primary/events/e1": {http.StatusNoContent, ""},
	})
	stdout := f.runOK(t, "events", "delete", "e1", "--send-updates", "none")
	got := f.last(t, "DELETE", "/calendar/v3/calendars/primary/events/e1")
	if !strings.Contains(got.Query, "sendUpdates=none") {
		t.Errorf("query = %q, want sendUpdates=none", got.Query)
	}
	if !strings.Contains(stdout, "deleted event e1") {
		t.Errorf("human output = %q, want the delete summary", stdout)
	}
}

// TestEventsRespond_PreservesOtherAttendees is the regression for the array-
// replacement trap: respond must fetch every attendee, flip only self, and
// write the full list back so the other guests are not dropped.
func TestEventsRespond_PreservesOtherAttendees(t *testing.T) {
	event := `{"id":"e1","attendees":[` +
		`{"email":"organizer@example.com","organizer":true,"responseStatus":"accepted"},` +
		`{"email":"other@example.com","responseStatus":"needsAction","displayName":"Other Guest"},` +
		`{"email":"me@example.com","self":true,"responseStatus":"needsAction"}` +
		`]}`
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/calendars/primary/events/e1":   {http.StatusOK, event},
		"PATCH /calendar/v3/calendars/primary/events/e1": {http.StatusOK, `{"id":"e1","summary":"Meeting"}`},
	})
	f.runOK(t, "events", "respond", "e1", "--status", "accepted", "--comment", "See you there")
	got := f.last(t, "PATCH", "/calendar/v3/calendars/primary/events/e1")

	// RSVP always notifies the organizer.
	if !strings.Contains(got.Query, "sendUpdates=all") {
		t.Errorf("query = %q, want sendUpdates=all for RSVP", got.Query)
	}
	var payload struct {
		Attendees []struct {
			Email          string `json:"email"`
			Self           bool   `json:"self"`
			ResponseStatus string `json:"responseStatus"`
			DisplayName    string `json:"displayName"`
			Comment        string `json:"comment"`
		} `json:"attendees"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if len(payload.Attendees) != 3 {
		t.Fatalf("patched %d attendees, want all 3 preserved", len(payload.Attendees))
	}
	var self, other bool
	for _, a := range payload.Attendees {
		if a.Self {
			self = true
			if a.ResponseStatus != "accepted" || a.Comment != "See you there" {
				t.Errorf("self attendee = %+v, want accepted + comment", a)
			}
		}
		if a.Email == "other@example.com" {
			other = true
			if a.ResponseStatus != "needsAction" || a.DisplayName != "Other Guest" {
				t.Errorf("other attendee = %+v, want unchanged status + displayName", a)
			}
		}
	}
	if !self || !other {
		t.Errorf("attendees missing self=%t or other=%t", self, other)
	}
}

func TestEventsRespond_NotAnAttendee(t *testing.T) {
	event := `{"id":"e1","attendees":[{"email":"someone@example.com","responseStatus":"accepted"}]}`
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/calendars/primary/events/e1": {http.StatusOK, event},
	})
	result, _, stderr := f.run(t, "events", "respond", "e1", "--status", "declined")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "not an attendee") {
		t.Errorf("stderr = %q, want the not-an-attendee error", stderr)
	}
	// The GET happened, but no PATCH must be sent.
	for _, r := range f.requests {
		if r.Method == http.MethodPatch {
			t.Errorf("a PATCH was sent for a non-attendee respond")
		}
	}
}
