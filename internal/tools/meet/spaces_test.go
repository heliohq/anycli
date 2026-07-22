package meet

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSpacesGet_ByMeetingCodeAlias(t *testing.T) {
	body := `{"name":"spaces/abc","meetingUri":"https://meet.google.com/abc-mnop-xyz","meetingCode":"abc-mnop-xyz","config":{"accessType":"TRUSTED","artifactConfig":{"transcriptionConfig":{"autoTranscriptionGeneration":"ON"}}}}`
	f := newFixture(t, map[string]route{
		"GET /v2/spaces/abc-mnop-xyz": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "spaces", "get", "abc-mnop-xyz")
	for _, want := range []string{"https://meet.google.com/abc-mnop-xyz", "AccessType:      TRUSTED", "AutoTranscript:  ON"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output = %q, want %q", stdout, want)
		}
	}
}

func TestSpacesCreate_ConfigPayload(t *testing.T) {
	body := `{"name":"spaces/new1","meetingUri":"https://meet.google.com/new-code","meetingCode":"new-code"}`
	f := newFixture(t, map[string]route{
		"POST /v2/spaces": {http.StatusOK, body},
	})
	stdout := f.runOK(t, "spaces", "create", "--access-type", "restricted", "--auto-transcription", "on", "--auto-recording", "off")
	if !strings.Contains(stdout, "https://meet.google.com/new-code") {
		t.Errorf("human output = %q, want the meeting uri", stdout)
	}
	got := f.last(t, "POST", "/v2/spaces")
	var payload struct {
		Config struct {
			AccessType     string `json:"accessType"`
			ArtifactConfig struct {
				RecordingConfig struct {
					AutoRecordingGeneration string `json:"autoRecordingGeneration"`
				} `json:"recordingConfig"`
				TranscriptionConfig struct {
					AutoTranscriptionGeneration string `json:"autoTranscriptionGeneration"`
				} `json:"transcriptionConfig"`
			} `json:"artifactConfig"`
		} `json:"config"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if payload.Config.AccessType != "RESTRICTED" {
		t.Errorf("accessType = %q, want RESTRICTED", payload.Config.AccessType)
	}
	if payload.Config.ArtifactConfig.TranscriptionConfig.AutoTranscriptionGeneration != "ON" {
		t.Errorf("autoTranscriptionGeneration = %q, want ON", payload.Config.ArtifactConfig.TranscriptionConfig.AutoTranscriptionGeneration)
	}
	if payload.Config.ArtifactConfig.RecordingConfig.AutoRecordingGeneration != "OFF" {
		t.Errorf("autoRecordingGeneration = %q, want OFF", payload.Config.ArtifactConfig.RecordingConfig.AutoRecordingGeneration)
	}
}

func TestSpacesCreate_NoFlagsEmptyBody(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v2/spaces": {http.StatusOK, `{"name":"spaces/n","meetingUri":"https://meet.google.com/x","meetingCode":"x"}`},
	})
	f.runOK(t, "spaces", "create")
	got := f.last(t, "POST", "/v2/spaces")
	if strings.TrimSpace(string(got.Body)) != "{}" {
		t.Errorf("request body = %q, want an empty object (org defaults)", got.Body)
	}
}

func TestSpacesUpdate_UpdateMask(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PATCH /v2/spaces/abc": {http.StatusOK, `{"name":"spaces/abc"}`},
	})
	stdout := f.runOK(t, "spaces", "update", "abc", "--auto-transcription", "on", "--moderation", "on")
	if !strings.Contains(stdout, "updated spaces/abc") {
		t.Errorf("human output = %q, want the update summary", stdout)
	}
	got := f.last(t, "PATCH", "/v2/spaces/abc")
	q, _ := decodeQuery(got.Query)
	mask := q["updateMask"]
	for _, want := range []string{
		"config.artifactConfig.transcriptionConfig.autoTranscriptionGeneration",
		"config.moderation",
	} {
		if !strings.Contains(mask, want) {
			t.Errorf("updateMask = %q, want it to contain %q", mask, want)
		}
	}
	if strings.Contains(mask, "accessType") {
		t.Errorf("updateMask = %q, must not contain unset fields", mask)
	}
}

func TestSpacesEndConference(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v2/spaces/abc:endActiveConference": {http.StatusOK, ``},
	})
	stdout := f.runOK(t, "spaces", "end-conference", "abc")
	if !strings.Contains(stdout, "ended active conference in spaces/abc") {
		t.Errorf("human output = %q, want the end summary", stdout)
	}

	// --json on an empty 200 body must still emit valid JSON.
	stdout = f.runOK(t, "spaces", "end-conference", "abc", "--json")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("--json output is not valid JSON: %v", err)
	}
}

// TestValueValidatorsAreStrictLowercase pins the fail-closed contract for
// value-conditioned policy (design 318 §equals audit rule): non-canonical
// spellings fail at command validation instead of executing while bypassing
// an equals condition on the literal argv value.
func TestValueValidatorsAreStrictLowercase(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
		want    string
	}{
		{"access-type canonical", "open", false, "OPEN"},
		{"access-type uppercase rejected", "OPEN", true, ""},
		{"access-type mixed case rejected", "Trusted", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := accessTypeValue(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accessTypeValue(%q) = %q, want strict-match error", tc.in, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("accessTypeValue(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
		})
	}

	onOffCases := []struct {
		name    string
		in      string
		wantErr bool
		want    string
	}{
		{"on canonical", "on", false, "ON"},
		{"uppercase rejected", "ON", true, ""},
		{"mixed case rejected", "Off", true, ""},
	}
	for _, tc := range onOffCases {
		t.Run("on-off "+tc.name, func(t *testing.T) {
			got, err := onOffValue("auto-recording", tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("onOffValue(%q) = %q, want strict-match error", tc.in, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("onOffValue(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
		})
	}
}
