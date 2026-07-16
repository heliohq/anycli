package calendar

import (
	"net/http"
	"strings"
	"testing"
)

func TestEventsList_QueryParams(t *testing.T) {
	body := `{"items":[{"id":"e1","summary":"Standup","status":"confirmed","start":{"dateTime":"2026-07-16T09:00:00-07:00"}}],"nextPageToken":"npt-9"}`
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/calendars/primary/events": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "events", "list",
		"--query", "standup", "--from", "2026-07-16T00:00:00-07:00", "--to", "2026-07-17T00:00:00-07:00",
		"--max", "5", "--single-events", "--order-by", "startTime")
	got := f.last(t, "GET", "/calendar/v3/calendars/primary/events")
	for _, param := range []string{"q=standup", "timeMin=2026-07-16T00%3A00%3A00-07%3A00", "timeMax=2026-07-17T00%3A00%3A00-07%3A00", "maxResults=5", "singleEvents=true", "orderBy=startTime"} {
		if !strings.Contains(got.Query, param) {
			t.Errorf("query = %q, want %q", got.Query, param)
		}
	}
	if !strings.Contains(stdout, "e1") || !strings.Contains(stdout, "Standup") || !strings.Contains(stdout, "next page token: npt-9") {
		t.Errorf("human output = %q, want id + title + next page token", stdout)
	}
}

func TestEventsList_CustomCalendarEscaped(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/calendars/team@group.calendar.google.com/events": {http.StatusOK, `{"items":[]}`},
	})
	stdout := f.runOK(t, "events", "list", "--calendar", "team@group.calendar.google.com")
	if !strings.Contains(stdout, "no events") {
		t.Errorf("human output = %q, want the empty marker", stdout)
	}
}

func TestEventsGet_JSONPassthrough(t *testing.T) {
	body := `{"id":"e1","summary":"1:1","status":"confirmed","start":{"dateTime":"2026-07-16T09:00:00-07:00"}}`
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/calendars/primary/events/e1": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "events", "get", "e1", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestEventsInstances(t *testing.T) {
	body := `{"items":[{"id":"e1_20260721","summary":"Weekly","start":{"dateTime":"2026-07-21T09:00:00-07:00"}}]}`
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/calendars/primary/events/e1/instances": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "events", "instances", "e1", "--from", "2026-07-20T00:00:00-07:00")
	got := f.last(t, "GET", "/calendar/v3/calendars/primary/events/e1/instances")
	if !strings.Contains(got.Query, "timeMin=2026-07-20T00%3A00%3A00-07%3A00") {
		t.Errorf("query = %q, want timeMin", got.Query)
	}
	if !strings.Contains(stdout, "e1_20260721") {
		t.Errorf("human output = %q, want the instance id", stdout)
	}
}

func TestEventsList_AllDayHumanMarker(t *testing.T) {
	body := `{"items":[{"id":"e2","summary":"Holiday","start":{"date":"2026-07-04"}}]}`
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/calendars/primary/events": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "events", "list")
	if !strings.Contains(stdout, "2026-07-04 (all-day)") {
		t.Errorf("human output = %q, want the all-day marker", stdout)
	}
}
