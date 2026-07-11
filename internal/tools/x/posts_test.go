package x

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPostGetAndSearch(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/2/tweets/123" {
				t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			}
			if r.URL.Query().Get("tweet.fields") != defaultPostFields {
				t.Fatalf("tweet.fields = %q", r.URL.Query().Get("tweet.fields"))
			}
			jsonResponse(w, http.StatusOK, `{"data":{"id":"123","text":"hello"}}`)
		})
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "post", "get", "123")
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
	})

	t.Run("search one page", func(t *testing.T) {
		server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/2/tweets/search/recent" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			want := url.Values{
				"query":        {"from:helio"},
				"max_results":  {"50"},
				"next_token":   {"next"},
				"tweet.fields": {defaultPostFields},
			}
			if r.URL.Query().Encode() != want.Encode() {
				t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
			}
			jsonResponse(w, http.StatusOK, `{"data":[],"meta":{"next_token":"another"}}`)
		})
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "post", "search", "--query", "from:helio", "--limit", "50", "--next-token", "next")
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
	})
}

func TestPostCreateWithMediaAndReply(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusCreated, `{"data":{"id":"999","text":"hello"}}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "post", "create", "--text", "hello", "--media-id", "11", "--media-id", "22", "--reply-to", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/2/tweets" || got.ContentType != "application/json" {
		t.Fatalf("request = %s %s %s", got.Method, got.Path, got.ContentType)
	}
	var payload struct {
		Text  string `json:"text"`
		Media struct {
			MediaIDs []string `json:"media_ids"`
		} `json:"media"`
		Reply struct {
			InReplyTo string `json:"in_reply_to_tweet_id"`
		} `json:"reply"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if payload.Text != "hello" || strings.Join(payload.Media.MediaIDs, ",") != "11,22" || payload.Reply.InReplyTo != "123" {
		t.Fatalf("payload = %+v", payload)
	}
	if stdout != `{"data":{"id":"999","text":"hello"}}`+"\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPostReply(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		reply := payload["reply"].(map[string]any)
		if reply["in_reply_to_tweet_id"] != "123" || payload["text"] != "reply" {
			t.Fatalf("payload = %#v", payload)
		}
		jsonResponse(w, http.StatusCreated, `{"data":{"id":"124"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "reply", "123", "--text", "reply")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestPostThreadRepliesSequentially(t *testing.T) {
	var requests []map[string]any
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		requests = append(requests, payload)
		id := "101"
		if len(requests) == 2 {
			id = "102"
		}
		jsonResponse(w, http.StatusCreated, `{"data":{"id":"`+id+`"}}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "post", "thread", "--text", "first", "--text", "second")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if _, ok := requests[0]["reply"]; ok {
		t.Fatalf("first post unexpectedly has reply: %#v", requests[0])
	}
	reply := requests[1]["reply"].(map[string]any)
	if reply["in_reply_to_tweet_id"] != "101" {
		t.Fatalf("second reply target = %v", reply["in_reply_to_tweet_id"])
	}
	if stdout != "{\"data\":{\"id\":\"101\"}}\n{\"data\":{\"id\":\"102\"}}\n" {
		t.Fatalf("stdout = %q, want provider-response JSONL", stdout)
	}
}

func TestPostThreadRequiresMultipleTexts(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "post", "thread", "--text", "only one")
	if code == 0 || !strings.Contains(stderr, "at least two --text") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestPostDelete(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/2/tweets/123" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"deleted":true}}`)
	})
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "post", "delete", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestRepostCreateAndDeleteUseConnectedUser(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		method     string
		path       string
		wantBody   string
		statusCode int
	}{
		{name: "create", args: []string{"repost", "create", "123"}, method: http.MethodPost, path: "/2/users/2244994945/retweets", wantBody: `{"tweet_id":"123"}`, statusCode: http.StatusCreated},
		{name: "delete", args: []string{"repost", "delete", "123"}, method: http.MethodDelete, path: "/2/users/2244994945/retweets/123", statusCode: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				got := captureRequest(t, r)
				if got.Method != tt.method || got.Path != tt.path {
					t.Fatalf("request = %s %s", got.Method, got.Path)
				}
				if tt.wantBody != "" && string(got.Body) != tt.wantBody {
					t.Fatalf("body = %s, want %s", got.Body, tt.wantBody)
				}
				jsonResponse(w, tt.statusCode, `{"data":{"retweeted":true}}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestRepostRequiresConnectedUserID(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, map[string]string{EnvAccessToken: "token"}, "repost", "create", "123")
	if code == 0 || !strings.Contains(stderr, "X_USER_ID is not set") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
