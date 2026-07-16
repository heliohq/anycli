package calendar

import (
	"net/http"
	"strings"
	"testing"
)

func TestCalendarsList_HumanAndJSON(t *testing.T) {
	body := `{"items":[{"id":"primary","summary":"Ada","timeZone":"America/Los_Angeles","accessRole":"owner","primary":true},{"id":"team@group.calendar.google.com","summary":"Team","timeZone":"UTC","accessRole":"writer"}],"nextPageToken":"npt-2"}`
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/users/me/calendarList": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "calendars", "list", "--max", "50")
	got := f.last(t, "GET", "/calendar/v3/users/me/calendarList")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}
	if !strings.Contains(got.Query, "maxResults=50") {
		t.Errorf("query = %q, want maxResults=50", got.Query)
	}
	for _, want := range []string{"primary", "owner", "(primary)", "Team", "writer", "next page token: npt-2"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}

	stdout = f.runOK(t, "calendars", "list", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestCalendarsGet(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /calendar/v3/users/me/calendarList/team@group.calendar.google.com": {http.StatusOK, `{"id":"team@group.calendar.google.com","summary":"Team","timeZone":"UTC","accessRole":"reader"}`},
	})
	stdout := f.runOK(t, "calendars", "get", "team@group.calendar.google.com")
	if !strings.Contains(stdout, "reader") || !strings.Contains(stdout, "tz=UTC") {
		t.Errorf("human output = %q, want access role + time zone", stdout)
	}
}
