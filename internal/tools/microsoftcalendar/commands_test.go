package microsoftcalendar

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCalendarsList_HumanJSONAndAuth(t *testing.T) {
	body := `{"value":[{"id":"c1","name":"Calendar","isDefaultCalendar":true,"owner":{"address":"me@example.com"}}]}`
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/calendars": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "calendars", "list")
	if !strings.Contains(stdout, "c1") || !strings.Contains(stdout, "(default)") {
		t.Errorf("human output = %q, want id + default marker", stdout)
	}
	got := f.last(t, "GET", "/v1.0/me/calendars")
	if got.Auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}

	stdout = f.runOK(t, "calendars", "list", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestEventsList_UsesEventsWithoutWindow(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/events": {http.StatusOK, `{"value":[{"id":"e1","subject":"Sync","start":{"dateTime":"2026-07-20T15:00:00.0000000","timeZone":"UTC"},"end":{"dateTime":"2026-07-20T15:30:00.0000000","timeZone":"UTC"}}]}`},
	})
	stdout := f.runOK(t, "events", "list", "--max", "5")
	if !strings.Contains(stdout, "e1") || !strings.Contains(stdout, "Sync") {
		t.Errorf("human output = %q, want id + subject", stdout)
	}
	got := f.last(t, "GET", "/v1.0/me/events")
	if !strings.Contains(got.Query, "%24top=5") {
		t.Errorf("query = %q, want $top=5", got.Query)
	}
}

func TestEventsList_WindowUsesCalendarView(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/calendarView": {http.StatusOK, `{"value":[]}`},
	})
	stdout := f.runOK(t, "events", "list", "--start", "2026-07-20T00:00:00Z", "--end", "2026-07-21T00:00:00Z", "--filter", "isAllDay eq false")
	if !strings.Contains(stdout, "no events") {
		t.Errorf("human output = %q, want the empty message", stdout)
	}
	got := f.last(t, "GET", "/v1.0/me/calendarView")
	if !strings.Contains(got.Query, "startDateTime=") || !strings.Contains(got.Query, "endDateTime=") {
		t.Errorf("query = %q, want start/end datetime params", got.Query)
	}
	if !strings.Contains(got.Query, "isAllDay") {
		t.Errorf("query = %q, want the $filter passed through", got.Query)
	}
}

func TestEventsList_PageFollowsNextLink(t *testing.T) {
	// The nextLink is an absolute URL; the client must GET it verbatim.
	next := "" // set below after server is up
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/events/next": {http.StatusOK, `{"value":[]}`},
	})
	next = f.srv.URL + "/v1.0/me/events/next?%24skiptoken=abc"
	f.runOK(t, "events", "list", "--page", next)
	got := f.last(t, "GET", "/v1.0/me/events/next")
	if !strings.Contains(got.Query, "skiptoken") {
		t.Errorf("query = %q, want the nextLink query preserved", got.Query)
	}
}

func TestEventsGet_Human(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/events/e1": {http.StatusOK, `{"id":"e1","subject":"Review","start":{"dateTime":"2026-07-20T15:00:00.0000000"},"end":{"dateTime":"2026-07-20T16:00:00.0000000"},"location":{"displayName":"Room 1"}}`},
	})
	stdout := f.runOK(t, "events", "get", "e1")
	if !strings.Contains(stdout, "Review") || !strings.Contains(stdout, "Room 1") {
		t.Errorf("human output = %q, want subject + location", stdout)
	}
}

func TestEventsCreate_PostsPayload(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/events": {http.StatusCreated, `{"id":"e9","start":{"dateTime":"2026-07-20T15:00:00.0000000"},"end":{"dateTime":"2026-07-20T15:30:00.0000000"}}`},
	})
	stdout := f.runOK(t, "events", "create",
		"--subject", "Plan", "--start", "2026-07-20T15:00:00", "--end", "2026-07-20T15:30:00",
		"--attendees", "a@b.com", "--location", "HQ", "--online")
	if !strings.Contains(stdout, "created event e9") {
		t.Errorf("human output = %q, want created confirmation", stdout)
	}
	got := f.last(t, "POST", "/v1.0/me/events")
	var payload map[string]any
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if payload["subject"] != "Plan" {
		t.Errorf("payload subject = %v, want Plan", payload["subject"])
	}
	if _, ok := payload["attendees"]; !ok {
		t.Errorf("payload = %v, want attendees present", payload)
	}
	if payload["isOnlineMeeting"] != true {
		t.Errorf("payload = %v, want isOnlineMeeting true", payload)
	}
	start, _ := payload["start"].(map[string]any)
	if start["timeZone"] != "UTC" || start["dateTime"] != "2026-07-20T15:00:00" {
		t.Errorf("start = %v, want UTC dateTime", start)
	}
}

func TestEventsUpdate_PatchesPayload(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /v1.0/me/events/e1": {http.StatusOK, `{"id":"e1"}`},
	})
	stdout := f.runOK(t, "events", "update", "e1", "--subject", "New", "--start", "2026-07-20T16:00:00")
	if !strings.Contains(stdout, "updated event e1") {
		t.Errorf("human output = %q, want update confirmation", stdout)
	}
	got := f.last(t, "PATCH", "/v1.0/me/events/e1")
	var payload map[string]any
	_ = json.Unmarshal(got.Body, &payload)
	if payload["subject"] != "New" {
		t.Errorf("payload = %v, want subject New", payload)
	}
	if _, ok := payload["start"]; !ok {
		t.Errorf("payload = %v, want start present", payload)
	}
}

func TestEventsUpdate_NothingToUpdate(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "events", "update", "e1")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "nothing to update") {
		t.Errorf("stderr = %q, want the nothing-to-update error", stderr)
	}
	if len(f.requests) != 0 {
		t.Errorf("must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestEventsCancel_NotifiesAttendees(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1.0/me/events/e1/cancel": {http.StatusAccepted, ``},
	})
	stdout := f.runOK(t, "events", "cancel", "e1", "--comment", "sorry")
	if !strings.Contains(stdout, "cancelled event e1") {
		t.Errorf("human output = %q, want cancel confirmation", stdout)
	}
	got := f.last(t, "POST", "/v1.0/me/events/e1/cancel")
	var payload map[string]any
	_ = json.Unmarshal(got.Body, &payload)
	if payload["comment"] != "sorry" {
		t.Errorf("payload = %v, want comment", payload)
	}
}

func TestEventsRespond_MapsAction(t *testing.T) {
	cases := []struct {
		action string
		verb   string
	}{
		{"accept", "accept"},
		{"decline", "decline"},
		{"tentative", "tentativelyAccept"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"POST /v1.0/me/events/e1/" + tc.verb: {http.StatusAccepted, ``},
			})
			stdout := f.runOK(t, "events", "respond", "e1", "--action", tc.action, "--no-notify")
			if !strings.Contains(stdout, "responded "+tc.action) {
				t.Errorf("human output = %q, want respond confirmation", stdout)
			}
			got := f.last(t, "POST", "/v1.0/me/events/e1/"+tc.verb)
			var payload map[string]any
			_ = json.Unmarshal(got.Body, &payload)
			if payload["sendResponse"] != false {
				t.Errorf("payload = %v, want sendResponse false with --no-notify", payload)
			}
		})
	}
}

func TestFreebusy_MergesBusyWindows(t *testing.T) {
	// Two overlapping busy events plus one free event; merged into one window.
	body := `{"value":[
		{"showAs":"busy","start":{"dateTime":"2026-07-20T15:00:00.0000000"},"end":{"dateTime":"2026-07-20T16:00:00.0000000"}},
		{"showAs":"tentative","start":{"dateTime":"2026-07-20T15:30:00.0000000"},"end":{"dateTime":"2026-07-20T17:00:00.0000000"}},
		{"showAs":"free","start":{"dateTime":"2026-07-20T18:00:00.0000000"},"end":{"dateTime":"2026-07-20T19:00:00.0000000"}}
	]}`
	f := newFixture(t, map[string]route{
		"GET /v1.0/me/calendarView": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "freebusy", "--start", "2026-07-20T00:00:00Z", "--end", "2026-07-21T00:00:00Z")
	if strings.Count(stdout, "busy") != 1 {
		t.Errorf("human output = %q, want exactly one merged busy window", stdout)
	}
	if !strings.Contains(stdout, "2026-07-20T15:00:00.0000000 → 2026-07-20T17:00:00.0000000") {
		t.Errorf("human output = %q, want merged window 15:00→17:00", stdout)
	}

	stdout = f.runOK(t, "freebusy", "--start", "2026-07-20T00:00:00Z", "--end", "2026-07-21T00:00:00Z", "--json")
	var out struct {
		Busy []busyWindow `json:"busy"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("--json not parseable: %v", err)
	}
	if len(out.Busy) != 1 || out.Busy[0].End != "2026-07-20T17:00:00.0000000" {
		t.Errorf("busy = %v, want one merged window ending 17:00", out.Busy)
	}
}

func TestFreebusy_FollowsNextLinkAcrossPages(t *testing.T) {
	// A window spanning more than one $top page must accumulate every page:
	// dropping later pages would silently report busy slots as free.
	routes := map[string]route{
		"GET /v1.0/me/calendarView":       {}, // page-1 body set below (needs server URL)
		"GET /v1.0/me/calendarView-page2": {http.StatusOK, `{"value":[{"showAs":"busy","start":{"dateTime":"2026-07-20T18:00:00.0000000"},"end":{"dateTime":"2026-07-20T19:00:00.0000000"}}]}`},
	}
	f := newFixture(t, routes)
	page2 := f.srv.URL + "/v1.0/me/calendarView-page2"
	routes["GET /v1.0/me/calendarView"] = route{http.StatusOK,
		`{"value":[{"showAs":"busy","start":{"dateTime":"2026-07-20T09:00:00.0000000"},"end":{"dateTime":"2026-07-20T10:00:00.0000000"}}],"@odata.nextLink":"` + page2 + `"}`}

	stdout := f.runOK(t, "freebusy", "--start", "2026-07-20T00:00:00Z", "--end", "2026-07-21T00:00:00Z")
	if !strings.Contains(stdout, "2026-07-20T09:00:00.0000000 → 2026-07-20T10:00:00.0000000") {
		t.Errorf("output = %q, want the page-1 busy window", stdout)
	}
	if !strings.Contains(stdout, "2026-07-20T18:00:00.0000000 → 2026-07-20T19:00:00.0000000") {
		t.Errorf("output = %q, want the page-2 busy window (nextLink must be followed)", stdout)
	}
}
