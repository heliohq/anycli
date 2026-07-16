package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestFreebusy_MultiCalendarAndBusyIntervals(t *testing.T) {
	body := `{"kind":"calendar#freeBusy","calendars":{` +
		`"primary":{"busy":[{"start":"2026-07-16T10:00:00Z","end":"2026-07-16T11:00:00Z"}]},` +
		`"alice@example.com":{"errors":[{"domain":"global","reason":"notFound"}]}` +
		`}}`
	f := newFixture(t, map[string]route{
		"POST /calendar/v3/freeBusy": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "freebusy",
		"--calendar", "primary,alice@example.com",
		"--from", "2026-07-16T00:00:00Z", "--to", "2026-07-17T00:00:00Z")
	got := f.last(t, "POST", "/calendar/v3/freeBusy")

	var payload struct {
		TimeMin string                `json:"timeMin"`
		TimeMax string                `json:"timeMax"`
		Items   []struct{ ID string } `json:"items"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if payload.TimeMin != "2026-07-16T00:00:00Z" || payload.TimeMax != "2026-07-17T00:00:00Z" {
		t.Errorf("payload time window = %q/%q", payload.TimeMin, payload.TimeMax)
	}
	if len(payload.Items) != 2 || payload.Items[0].ID != "primary" || payload.Items[1].ID != "alice@example.com" {
		t.Errorf("items = %+v, want the two comma-split calendar ids", payload.Items)
	}
	if !strings.Contains(stdout, "busy: 2026-07-16T10:00:00Z → 2026-07-16T11:00:00Z") {
		t.Errorf("human output = %q, want the busy interval", stdout)
	}
	if !strings.Contains(stdout, "notFound") {
		t.Errorf("human output = %q, want the inline error surfaced", stdout)
	}
}

func TestFreebusy_TooManyCalendars(t *testing.T) {
	ids := make([]string, maxFreebusyCalendars+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("c%d@example.com", i)
	}
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "freebusy",
		"--calendar", strings.Join(ids, ","),
		"--from", "2026-07-16T00:00:00Z", "--to", "2026-07-17T00:00:00Z")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "too many calendars") {
		t.Errorf("stderr = %q, want the pre-flight cap message", stderr)
	}
	if len(f.requests) != 0 {
		t.Errorf("cap violation must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestFreebusy_JSONPassthrough(t *testing.T) {
	body := `{"kind":"calendar#freeBusy","calendars":{"primary":{"busy":[]}}}`
	f := newFixture(t, map[string]route{
		"POST /calendar/v3/freeBusy": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "freebusy", "--calendar", "primary", "--from", "2026-07-16T00:00:00Z", "--to", "2026-07-17T00:00:00Z", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}
