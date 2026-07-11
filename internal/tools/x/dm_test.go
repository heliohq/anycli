package x

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDMListAndGet(t *testing.T) {
	t.Run("list one page", func(t *testing.T) {
		server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/2/dm_events" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			want := url.Values{
				"dm_event.fields":  {defaultDMFields},
				"expansions":       {defaultDMExpansions},
				"max_results":      {"20"},
				"media.fields":     {defaultDMMediaFields},
				"pagination_token": {"page-2"},
			}
			if r.URL.Query().Encode() != want.Encode() {
				t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
			}
			jsonResponse(w, http.StatusOK, `{"data":[],"meta":{"next_token":"page-3"}}`)
		})
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "dm", "list", "--limit", "20", "--next-token", "page-2")
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
	})

	t.Run("get", func(t *testing.T) {
		server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/2/dm_events/99" {
				t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			}
			if r.URL.Query().Get("dm_event.fields") != defaultDMFields {
				t.Fatalf("dm_event.fields = %q", r.URL.Query().Get("dm_event.fields"))
			}
			if r.URL.Query().Get("expansions") != defaultDMExpansions || r.URL.Query().Get("media.fields") != defaultDMMediaFields {
				t.Fatalf("DM media expansion query = %q", r.URL.RawQuery)
			}
			jsonResponse(w, http.StatusOK, `{"data":{"id":"99"}}`)
		})
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "dm", "get", "99")
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
	})
}

func TestDMHistoryRequiresExactlyOneConversationSelector(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	for _, args := range [][]string{
		{"dm", "history"},
		{"dm", "history", "--conversation-id", "1", "--participant-id", "2"},
	} {
		code, _, stderr := run(t, server, fullEnv(), args...)
		if code == 0 || !strings.Contains(stderr, "exactly one of --conversation-id or --participant-id") {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestDMHistoryRoutesByConversationOrParticipant(t *testing.T) {
	tests := []struct {
		name string
		flag string
		path string
	}{
		{name: "conversation", flag: "--conversation-id", path: "/2/dm_conversations/123456789012345/dm_events"},
		{name: "participant", flag: "--participant-id", path: "/2/dm_conversations/with/20/dm_events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := "123456789012345"
			if tt.flag == "--participant-id" {
				value = "20"
			}
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, tt.path)
				}
				if r.URL.Query().Get("dm_event.fields") != defaultDMFields {
					t.Fatalf("dm_event.fields = %q", r.URL.Query().Get("dm_event.fields"))
				}
				if r.URL.Query().Get("expansions") != defaultDMExpansions || r.URL.Query().Get("media.fields") != defaultDMMediaFields {
					t.Fatalf("DM media expansion query = %q", r.URL.RawQuery)
				}
				jsonResponse(w, http.StatusOK, `{"data":[]}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), "dm", "history", tt.flag, value)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestDMConversationCreate(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/2/dm_conversations" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			ConversationType string   `json:"conversation_type"`
			ParticipantIDs   []string `json:"participant_ids"`
			Message          struct {
				Text        string `json:"text"`
				Attachments []struct {
					MediaID string `json:"media_id"`
				} `json:"attachments"`
			} `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.ConversationType != "Group" || strings.Join(payload.ParticipantIDs, ",") != "11,22" || payload.Message.Text != "welcome" || payload.Message.Attachments[0].MediaID != "33" {
			t.Fatalf("payload = %+v", payload)
		}
		jsonResponse(w, http.StatusCreated, `{"data":{"dm_conversation_id":"100","dm_event_id":"101"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "dm", "conversation", "create", "--participant-id", "11", "--participant-id", "22", "--text", "welcome", "--media-id", "33")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestDMSendRoutesByExactlyOneTarget(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{name: "participant", args: []string{"dm", "send", "--participant-id", "20", "--text", "hello"}, path: "/2/dm_conversations/with/20/messages"},
		{name: "conversation", args: []string{"dm", "send", "--conversation-id", "123456789012345", "--media-id", "30"}, path: "/2/dm_conversations/123456789012345/messages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tt.path {
					t.Fatalf("request = %s %s, want POST %s", r.Method, r.URL.Path, tt.path)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload["text"] == nil && payload["attachments"] == nil {
					t.Fatalf("payload contains neither text nor media: %#v", payload)
				}
				jsonResponse(w, http.StatusCreated, `{"data":{"dm_event_id":"101"}}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestDMConversationIDValidation(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	for _, args := range [][]string{
		{"dm", "history", "--conversation-id", "10"},
		{"dm", "send", "--conversation-id", "not-a-conversation", "--text", "hello"},
	} {
		code, _, stderr := run(t, server, fullEnv(), args...)
		if code == 0 || !strings.Contains(stderr, "conversation id must be") {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestDMConversationIDAcceptsOfficialFormats(t *testing.T) {
	for _, id := range []string{"123456789012345", "123-456"} {
		if err := requireDMConversationID(id); err != nil {
			t.Errorf("requireDMConversationID(%q) = %v", id, err)
		}
	}
}

func TestDMSendValidatesTargetAndContent(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"dm", "send", "--text", "hello"}, want: "exactly one of --conversation-id or --participant-id"},
		{args: []string{"dm", "send", "--participant-id", "20"}, want: "at least one of --text or --media-id"},
		{args: []string{"dm", "send", "--participant-id", "20", "--conversation-id", "10", "--text", "hello"}, want: "exactly one of --conversation-id or --participant-id"},
	}
	for _, tt := range tests {
		code, _, stderr := run(t, server, fullEnv(), tt.args...)
		if code == 0 || !strings.Contains(stderr, tt.want) {
			t.Fatalf("args=%v code=%d stderr=%q want=%q", tt.args, code, stderr, tt.want)
		}
	}
}

func TestDMDelete(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/2/dm_events/99" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"deleted":true}}`)
	})
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "dm", "delete", "99")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestDMMediaDownloadWritesBinaryFile(t *testing.T) {
	output := filepath.Join(t.TempDir(), "photo.jpg")
	binary := []byte{0xff, 0xd8, 0xff, 0xe0, 0x01, 0x02}
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/2/dm_conversations/media/99/88/photo.jpg" {
			t.Fatalf("request = %s %s", r.Method, r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(binary)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "dm", "media", "download", "--event-id", "99", "--media-key", "3_88", "--resource-id", "photo.jpg", "--output", output)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(binary) {
		t.Fatalf("download = %v, want %v", got, binary)
	}
	if !strings.Contains(stdout, `"path"`) || !strings.Contains(stdout, `"bytes":6`) {
		t.Fatalf("stdout = %q, want JSON download result", stdout)
	}
}

func TestDMMediaDownloadValidatesMediaKey(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	output := filepath.Join(t.TempDir(), "photo.jpg")
	code, _, stderr := run(t, server, fullEnv(), "dm", "media", "download", "--event-id", "99", "--media-key", "not-a-key", "--resource-id", "photo.jpg", "--output", output)
	if code == 0 || !strings.Contains(stderr, "media key must") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
