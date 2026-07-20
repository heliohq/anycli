package x

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPostRepliesResolvesConversationThenSearches(t *testing.T) {
	var searchQuery url.Values
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/tweets/9":
			if r.URL.Query().Get("tweet.fields") != "conversation_id" {
				t.Fatalf("lookup tweet.fields = %q", r.URL.Query().Get("tweet.fields"))
			}
			jsonResponse(w, http.StatusOK, `{"data":{"id":"9","conversation_id":"100"}}`)
		case "/2/tweets/search/recent":
			searchQuery = r.URL.Query()
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"101","text":"a reply"}]}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "post", "replies", "9", "--limit", "25", "--since-id", "5")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := url.Values{
		"query":        {"conversation_id:100"},
		"max_results":  {"25"},
		"since_id":     {"5"},
		"tweet.fields": {defaultPostFields},
	}
	if searchQuery.Encode() != want.Encode() {
		t.Fatalf("search query = %q, want %q", searchQuery.Encode(), want.Encode())
	}
	if stdout != `{"data":[{"id":"101","text":"a reply"}]}`+"\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPostRepliesPassesNextToken(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/tweets/9":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"9","conversation_id":"100"}}`)
		case "/2/tweets/search/recent":
			if r.URL.Query().Get("next_token") != "page2" {
				t.Fatalf("next_token = %q", r.URL.Query().Get("next_token"))
			}
			jsonResponse(w, http.StatusOK, `{"data":[]}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "replies", "9", "--next-token", "page2")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestPostRepliesFailsWithoutConversationID(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, `{"data":{"id":"9"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "replies", "9")
	if code == 0 || !strings.Contains(stderr, "conversation_id") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestPostQuotes(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/2/tweets/123/quote_tweets" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		want := url.Values{
			"max_results":      {"20"},
			"pagination_token": {"next"},
			"tweet.fields":     {defaultPostFields},
		}
		if r.URL.Query().Encode() != want.Encode() {
			t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
		}
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "quotes", "123", "--limit", "20", "--next-token", "next")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestPostQuoteCreatesQuotePost(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusCreated, `{"data":{"id":"777"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "quote", "123", "--text", "look at this")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/2/tweets" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	var payload struct {
		Text         string `json:"text"`
		QuoteTweetID string `json:"quote_tweet_id"`
	}
	if err := json.Unmarshal(got.Body, &payload); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if payload.Text != "look at this" || payload.QuoteTweetID != "123" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestPostHideAndUnhide(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantBody string
	}{
		{name: "hide", args: []string{"post", "hide", "456"}, wantBody: `{"hidden":true}`},
		{name: "unhide", args: []string{"post", "unhide", "456"}, wantBody: `{"hidden":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				got := captureRequest(t, r)
				if got.Method != http.MethodPut || got.Path != "/2/tweets/456/hidden" {
					t.Fatalf("request = %s %s", got.Method, got.Path)
				}
				if string(got.Body) != tt.wantBody {
					t.Fatalf("body = %s, want %s", got.Body, tt.wantBody)
				}
				jsonResponse(w, http.StatusOK, `{"data":{"hidden":true}}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}
