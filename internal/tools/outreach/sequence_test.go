package outreach

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSequenceStepsFiltersBySequence(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "sequence", "steps", "12")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/sequenceSteps" {
		t.Fatalf("path = %q, want /sequenceSteps", got.Path)
	}
	if !strings.Contains(got.RawQuery, "filter[sequence][id]=12") {
		t.Fatalf("query = %q, want sequence filter", got.RawQuery)
	}
}

func TestEnrollmentAddBuildsThreeRelationships(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusCreated, `{"data":{"type":"sequenceState","id":"77"}}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(),
		"enrollment", "add", "--prospect-id", "1", "--sequence-id", "2", "--mailbox-id", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/sequenceStates" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	var body struct {
		Data struct {
			Type          string `json:"type"`
			Relationships map[string]struct {
				Data linkage `json:"data"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.Type != "sequenceState" {
		t.Fatalf("type = %q", body.Data.Type)
	}
	wantRel := map[string]linkage{
		"prospect": {Type: "prospect", ID: "1"},
		"sequence": {Type: "sequence", ID: "2"},
		"mailbox":  {Type: "mailbox", ID: "3"},
	}
	for name, want := range wantRel {
		got := body.Data.Relationships[name].Data
		if got != want {
			t.Fatalf("relationship %q = %+v, want %+v", name, got, want)
		}
	}
	if stdout != `{"id":"77","type":"sequenceState"}`+"\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestEnrollmentAddRequiresAllThreeIDs(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "enrollment", "add", "--prospect-id", "1", "--sequence-id", "2")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "mailbox-id") {
		t.Fatalf("stderr = %q, want required-flag error", stderr)
	}
}

func TestEnrollmentActionsPostToActionsPath(t *testing.T) {
	for _, action := range []string{"pause", "resume", "finish"} {
		t.Run(action, func(t *testing.T) {
			var got capturedRequest
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				got = captureRequest(t, r)
				jsonResponse(w, http.StatusOK, `{"data":{"type":"sequenceState","id":"77"}}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), "enrollment", action, "77")
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
			wantPath := "/sequenceStates/77/actions/" + action
			if got.Method != http.MethodPost || got.Path != wantPath {
				t.Fatalf("request = %s %s, want POST %s", got.Method, got.Path, wantPath)
			}
		})
	}
}
