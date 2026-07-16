package meet

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordingsList(t *testing.T) {
	body := `{"recordings":[{"name":"conferenceRecords/r1/recordings/rec1","state":"FILE_GENERATED","driveDestination":{"file":"fileABC","exportUri":"https://drive.google.com/file/d/fileABC/view"}}]}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/recordings": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "recordings", "list", "r1")
	for _, want := range []string{"FILE_GENERATED", "fileABC", "https://drive.google.com/file/d/fileABC/view"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}
	// Requests the 100 cap so the provider default of 10 can't silently truncate.
	q, _ := decodeQuery(f.last(t, "GET", "/v2/conferenceRecords/r1/recordings").Query)
	if q["pageSize"] != "100" {
		t.Errorf("recordings pageSize = %q, want 100", q["pageSize"])
	}
}

// TestArtifactsList_TruncationHint asserts the single-page artifact lists warn
// (rather than silently drop) when the provider returns a nextPageToken.
func TestArtifactsList_TruncationHint(t *testing.T) {
	body := `{"recordings":[{"name":"conferenceRecords/r1/recordings/rec1","state":"FILE_GENERATED"}],"nextPageToken":"more"}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/recordings": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "recordings", "list", "r1")
	if !strings.Contains(stdout, "some were not shown") {
		t.Errorf("human output = %q, want the truncation hint on a nextPageToken", stdout)
	}
}

func TestTranscriptsList_Empty(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/transcripts": {http.StatusOK, `{}`},
	})
	stdout := f.runOK(t, "transcripts", "list", "r1")
	if !strings.Contains(stdout, "no transcripts") {
		t.Errorf("human output = %q, want the empty notice (edition/未开转录 is not an error)", stdout)
	}
}

func TestTranscriptsEntries(t *testing.T) {
	body := `{"transcriptEntries":[{"name":"conferenceRecords/r1/transcripts/t1/entries/e1","participant":"conferenceRecords/r1/participants/p1","text":"Hello team","startTime":"2026-07-01T10:00:00Z"}],"nextPageToken":"n2"}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/transcripts/t1/entries": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "transcripts", "entries", "conferenceRecords/r1/transcripts/t1")
	if !strings.Contains(stdout, "Hello team") || !strings.Contains(stdout, "next page token: n2") {
		t.Errorf("human output = %q, want the entry text + next token", stdout)
	}
}

// TestTranscriptsText_SynthesisAndSpeakerJoin covers the synthetic verb: it
// pages the record's participants to resolve speaker names, pages every entry,
// orders by start time, and stitches `speaker: text` lines.
func TestTranscriptsText_SynthesisAndSpeakerJoin(t *testing.T) {
	participants := `{"participants":[
		{"name":"conferenceRecords/r1/participants/p1","signedinUser":{"user":"u1","displayName":"Alice"}},
		{"name":"conferenceRecords/r1/participants/p2","anonymousUser":{"displayName":"Guest"}}
	]}`
	// Two entries returned out of chronological order to exercise the sort.
	entries := `{"transcriptEntries":[
		{"participant":"conferenceRecords/r1/participants/p2","text":"Second line","startTime":"2026-07-01T10:01:00Z"},
		{"participant":"conferenceRecords/r1/participants/p1","text":"First line","startTime":"2026-07-01T10:00:00Z"}
	]}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/participants":           {http.StatusOK, participants},
		"GET /v2/conferenceRecords/r1/transcripts/t1/entries": {http.StatusOK, entries},
	})
	stdout := f.runOK(t, "transcripts", "text", "conferenceRecords/r1/transcripts/t1")
	if want := "Alice: First line\nGuest: Second line\n"; stdout != want {
		t.Errorf("text output = %q, want ordered speaker lines %q", stdout, want)
	}
	// Participants pagination requests the 250 cap.
	pq, _ := decodeQuery(f.last(t, "GET", "/v2/conferenceRecords/r1/participants").Query)
	if pq["pageSize"] != "250" {
		t.Errorf("participants pageSize = %q, want 250", pq["pageSize"])
	}
}

func TestTranscriptsText_JSONAndSave(t *testing.T) {
	participants := `{"participants":[{"name":"conferenceRecords/r1/participants/p1","signedinUser":{"displayName":"Alice"}}]}`
	entries := `{"transcriptEntries":[{"participant":"conferenceRecords/r1/participants/p1","text":"Hi","startTime":"2026-07-01T10:00:00Z"}]}`
	f := newFixture(t, map[string]route{
		"GET /v2/conferenceRecords/r1/participants":           {http.StatusOK, participants},
		"GET /v2/conferenceRecords/r1/transcripts/t1/entries": {http.StatusOK, entries},
	})

	stdout := f.runOK(t, "transcripts", "text", "conferenceRecords/r1/transcripts/t1", "--json")
	var parsed struct {
		Transcript string `json:"transcript"`
		Source     string `json:"source"`
		Lines      []struct {
			Speaker string `json:"speaker"`
			Text    string `json:"text"`
		} `json:"lines"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("--json output is not valid JSON: %v", err)
	}
	if parsed.Source != "meet-api-entries" || len(parsed.Lines) != 1 || parsed.Lines[0].Speaker != "Alice" {
		t.Errorf("--json = %+v, want source + one Alice line", parsed)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.txt")
	stdout = f.runOK(t, "transcripts", "text", "conferenceRecords/r1/transcripts/t1", "--save", path)
	if !strings.Contains(stdout, "saved 1 line(s)") {
		t.Errorf("stdout = %q, want the save summary", stdout)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(saved) != "Alice: Hi\n" {
		t.Errorf("saved file = %q, want the stitched line", saved)
	}
}

func TestSmartNotesList_HitsV2Beta(t *testing.T) {
	body := `{"smartNotes":[{"name":"conferenceRecords/r1/smartNotes/n1","state":"FILE_GENERATED","docsDestination":{"document":"docXYZ","exportUri":"https://docs.google.com/document/d/docXYZ"}}]}`
	f := newFixture(t, map[string]route{
		"GET /v2beta/conferenceRecords/r1/smartNotes": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "smart-notes", "list", "r1")
	for _, want := range []string{"FILE_GENERATED", "docXYZ", "https://docs.google.com/document/d/docXYZ"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}
	// Must route to the v2beta base, not v2.
	got := f.last(t, "GET", "/v2beta/conferenceRecords/r1/smartNotes")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token on the beta call", got.Auth)
	}
	q, _ := decodeQuery(got.Query)
	if q["pageSize"] != "100" {
		t.Errorf("smart notes pageSize = %q, want 100", q["pageSize"])
	}
}
