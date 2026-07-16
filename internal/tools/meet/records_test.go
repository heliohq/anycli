package meet

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRecordsList_FilterAssembly(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords": {http.StatusOK, `{"conferenceRecords":[{"name":"conferenceRecords/r1","startTime":"2026-07-01T10:00:00Z","endTime":"2026-07-01T11:00:00Z","space":"spaces/abc"}],"nextPageToken":"tok2"}`},
	})
	stdout := f.runOK(t, "records", "list", "--space", "abc-mnop-xyz", "--after", "2026-07-01T00:00:00Z", "--ongoing", "--filter", `space.name = "spaces/z"`, "--max", "50")
	if !strings.Contains(stdout, "conferenceRecords/r1") || !strings.Contains(stdout, "next page token: tok2") {
		t.Errorf("human output = %q, want the record and next token", stdout)
	}
	got := f.last(t, "GET", "/v2/conferenceRecords")
	q, err := decodeQuery(got.Query)
	if err != nil {
		t.Fatalf("bad query: %v", err)
	}
	filter := q["filter"]
	for _, want := range []string{
		`space.meeting_code = "abc-mnop-xyz"`,
		`start_time>="2026-07-01T00:00:00Z"`,
		"end_time IS NULL",
		`space.name = "spaces/z"`,
		" AND ",
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter = %q, want it to contain %q", filter, want)
		}
	}
	if q["pageSize"] != "50" {
		t.Errorf("pageSize = %q, want 50", q["pageSize"])
	}
}

func TestRecordsList_SpaceResourceNameFilter(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords": {http.StatusOK, `{"conferenceRecords":[]}`},
	})
	f.runOK(t, "records", "list", "--space", "spaces/abc")
	got := f.last(t, "GET", "/v2/conferenceRecords")
	q, _ := decodeQuery(got.Query)
	if q["filter"] != `space.name = "spaces/abc"` {
		t.Errorf("filter = %q, want a space.name clause for a resource name", q["filter"])
	}
}

func TestRecordsList_Empty(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords": {http.StatusOK, `{}`},
	})
	stdout := f.runOK(t, "records", "list")
	if !strings.Contains(stdout, "no conference records") {
		t.Errorf("human output = %q, want the empty notice", stdout)
	}
}

func TestRecordsGet_HumanAndJSON(t *testing.T) {
	body := `{"name":"conferenceRecords/r1","startTime":"2026-07-01T10:00:00Z","endTime":"2026-07-01T11:00:00Z","expireTime":"2026-07-31T11:00:00Z","space":"spaces/abc"}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "records", "get", "r1")
	for _, want := range []string{"conferenceRecords/r1", "Expires: 2026-07-31T11:00:00Z", "Space:   spaces/abc"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}
	if got := f.last(t, "GET", "/v2/conferenceRecords/r1"); got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}

	stdout = f.runOK(t, "records", "get", "conferenceRecords/r1", "--json")
	if strings.TrimSpace(stdout) != body {
		t.Errorf("--json output = %q, want the raw provider body", stdout)
	}
}

func TestParticipantsList_DisplayNameFallback(t *testing.T) {
	body := `{"participants":[
		{"name":"conferenceRecords/r1/participants/p1","earliestStartTime":"2026-07-01T10:00:00Z","latestEndTime":"2026-07-01T11:00:00Z","signedinUser":{"user":"u1","displayName":"Alice"}},
		{"name":"conferenceRecords/r1/participants/p2","earliestStartTime":"2026-07-01T10:05:00Z","anonymousUser":{"displayName":"Guest 12"}},
		{"name":"conferenceRecords/r1/participants/p3","earliestStartTime":"2026-07-01T10:06:00Z"}
	]}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/participants": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "participants", "list", "r1")
	for _, want := range []string{"Alice", "Guest 12", "conferenceRecords/r1/participants/p3", "still in call"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}
	got := f.last(t, "GET", "/v2/conferenceRecords/r1/participants")
	q, _ := decodeQuery(got.Query)
	if q["pageSize"] != "100" {
		t.Errorf("pageSize = %q, want the participants default 100", q["pageSize"])
	}
}

func TestParticipantsSessions(t *testing.T) {
	body := `{"participantSessions":[{"name":"conferenceRecords/r1/participants/p1/participantSessions/s1","startTime":"2026-07-01T10:00:00Z","endTime":"2026-07-01T10:30:00Z"},{"name":"conferenceRecords/r1/participants/p1/participantSessions/s2","startTime":"2026-07-01T10:32:00Z"}]}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/participants/p1/participantSessions": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "participants", "sessions", "conferenceRecords/r1/participants/p1")
	if !strings.Contains(stdout, "s1") || !strings.Contains(stdout, "still in call") {
		t.Errorf("human output = %q, want the two sessions", stdout)
	}

	stdout = f.runOK(t, "participants", "sessions", "conferenceRecords/r1/participants/p1", "--json")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("--json output is not valid JSON: %v", err)
	}
}
