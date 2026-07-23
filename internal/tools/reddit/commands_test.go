package reddit

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// decodeLines splits stdout into the JSONL objects the listing commands emit.
func decodeLines(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line is not JSON: %v (%q)", err, line)
		}
		out = append(out, m)
	}
	return out
}

const subredditPostsBody = `{"kind":"Listing","data":{"after":"t3_next","children":[
  {"kind":"t3","data":{"id":"p1","name":"t3_p1","title":"Hello","author":"alice","subreddit":"golang","score":42,"num_comments":7,"permalink":"/r/golang/comments/p1/hello/","selftext":"body & more"}}
]}}`

func TestSubredditPosts_DefaultSortStripsEnvelopeAndEmitsCursor(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, subredditPostsBody, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "subreddit", "posts", "golang", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/r/golang/hot" {
		t.Errorf("path = %q, want /r/golang/hot (default sort)", got.Path)
	}
	assertAuth(t, got)
	assertRawJSON(t, got)

	lines := decodeLines(t, stdout)
	if len(lines) != 2 {
		t.Fatalf("emitted %d lines, want 2 (post + cursor): %q", len(lines), stdout)
	}
	if lines[0]["fullname"] != "t3_p1" || lines[0]["title"] != "Hello" {
		t.Errorf("post = %+v, want flat item with fullname t3_p1", lines[0])
	}
	if _, ok := lines[0]["kind"]; ok {
		t.Errorf("post %+v still carries the kind/data envelope", lines[0])
	}
	if lines[1]["after"] != "t3_next" {
		t.Errorf("cursor = %+v, want {after: t3_next}", lines[1])
	}
}

func TestSubredditPosts_SortAndTimeInPathAndQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, subredditPostsBody, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "subreddit", "posts", "golang", "--sort", "top", "--time", "week", "--limit", "5")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/r/golang/top" {
		t.Errorf("path = %q, want /r/golang/top", got.Path)
	}
	if got.Query.Get("t") != "week" || got.Query.Get("limit") != "5" {
		t.Errorf("query = %v, want t=week limit=5", got.Query)
	}
}

func TestSubredditPosts_BadSortIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "subreddit", "posts", "golang", "--sort", "sideways")
	if code != 2 {
		t.Errorf("exit = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "--sort") {
		t.Errorf("stderr = %q, want a --sort usage error", stderr)
	}
}

func TestSearch_GlobalUsesLinkType(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, subredditPostsBody, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search", "--query", "golang generics", "--sort", "top")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/search" {
		t.Errorf("path = %q, want /search", got.Path)
	}
	if got.Query.Get("q") != "golang generics" || got.Query.Get("type") != "link" || got.Query.Get("sort") != "top" {
		t.Errorf("query = %v, want q + type=link + sort=top", got.Query)
	}
	if got.Query.Get("restrict_sr") != "" {
		t.Errorf("restrict_sr should be unset for a global search, got %q", got.Query.Get("restrict_sr"))
	}
}

func TestSearch_SubredditRestricted(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, subredditPostsBody, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "search", "--query", "generics", "--subreddit", "golang")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/r/golang/search" || got.Query.Get("restrict_sr") != "1" {
		t.Errorf("path=%q restrict_sr=%q, want /r/golang/search with restrict_sr=1", got.Path, got.Query.Get("restrict_sr"))
	}
}

func TestSearch_MissingQueryIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "search")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--query") {
		t.Errorf("stderr = %q, want a --query usage error", stderr)
	}
}

const commentTreeBody = `[
  {"kind":"Listing","data":{"children":[{"kind":"t3","data":{"id":"p1","name":"t3_p1","title":"Root"}}]}},
  {"kind":"Listing","data":{"after":null,"children":[
    {"kind":"t1","data":{"id":"c1","name":"t1_c1","author":"bob","body":"top-level","score":5,"parent_id":"t3_p1","replies":{"kind":"Listing","data":{"children":[
      {"kind":"t1","data":{"id":"c2","name":"t1_c2","author":"carol","body":"nested","score":2,"parent_id":"t1_c1","replies":""}}
    ]}}}},
    {"kind":"more","data":{"count":3,"parent_id":"t3_p1"}}
  ]}}
]`

func TestPostComments_FlattensTreeWithDepthAndMoreStub(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, commentTreeBody, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "post", "comments", "t3_p1", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/comments/p1" {
		t.Errorf("path = %q, want /comments/p1 (t3_ stripped)", got.Path)
	}
	lines := decodeLines(t, stdout)
	if len(lines) != 3 {
		t.Fatalf("emitted %d lines, want 3 (c1, c2, more): %q", len(lines), stdout)
	}
	if lines[0]["fullname"] != "t1_c1" || lines[0]["depth"].(float64) != 0 {
		t.Errorf("c1 = %+v, want fullname t1_c1 depth 0", lines[0])
	}
	if lines[1]["fullname"] != "t1_c2" || lines[1]["depth"].(float64) != 1 {
		t.Errorf("c2 = %+v, want fullname t1_c2 depth 1", lines[1])
	}
	if lines[2]["kind"] != "more" || lines[2]["count"].(float64) != 3 {
		t.Errorf("more = %+v, want {kind:more,count:3}", lines[2])
	}
}

func TestPostCreate_SelfPostFormAndEcho(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"json":{"errors":[],"data":{"id":"abc","name":"t3_abc","url":"https://www.reddit.com/r/golang/comments/abc/"}}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "post", "create", "--subreddit", "golang", "--title", "Hi", "--text", "body", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/api/submit" {
		t.Errorf("request = %s %s, want POST /api/submit", got.Method, got.Path)
	}
	if got.Form.Get("api_type") != "json" || got.Form.Get("kind") != "self" ||
		got.Form.Get("sr") != "golang" || got.Form.Get("title") != "Hi" || got.Form.Get("text") != "body" {
		t.Errorf("form = %v, want api_type=json kind=self sr=golang title=Hi text=body", got.Form)
	}
	if !strings.Contains(stdout, "t3_abc") {
		t.Errorf("stdout = %q, want the created fullname echoed", stdout)
	}
}

func TestPostCreate_LinkPostUsesKindLink(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"json":{"errors":[],"data":{"id":"x","name":"t3_x"}}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "post", "create", "--subreddit", "golang", "--title", "Hi", "--url", "https://go.dev")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Form.Get("kind") != "link" || got.Form.Get("url") != "https://go.dev" {
		t.Errorf("form = %v, want kind=link url=https://go.dev", got.Form)
	}
}

func TestPostCreate_TextAndURLMutuallyExclusive(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "post", "create", "--subreddit", "golang", "--title", "Hi", "--text", "a", "--url", "b")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "exactly one") {
		t.Errorf("stderr = %q, want the mutual-exclusion usage error", stderr)
	}
}

func TestPostCreate_JSONErrorsDialectIsExit1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"json":{"errors":[["SUBREDDIT_NOEXIST","that subreddit doesn't exist","sr"]],"data":{}}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "post", "create", "--subreddit", "nope", "--title", "Hi", "--text", "body")
	if code != 1 {
		t.Errorf("exit = %d, want 1 (200-with-errors dialect)", code)
	}
	if !strings.Contains(stderr, "SUBREDDIT_NOEXIST") {
		t.Errorf("stderr = %q, want the surfaced action error code", stderr)
	}
}

func TestCommentCreate_FormShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"json":{"errors":[],"data":{"things":[{"kind":"t1","data":{"id":"c9","name":"t1_c9","permalink":"/r/x/c9"}}]}}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "comment", "create", "--parent", "t3_p1", "--text", "nice", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/comment" || got.Form.Get("thing_id") != "t3_p1" || got.Form.Get("text") != "nice" || got.Form.Get("api_type") != "json" {
		t.Errorf("request = %s form=%v, want /api/comment thing_id=t3_p1 text=nice api_type=json", got.Path, got.Form)
	}
	if !strings.Contains(stdout, "t1_c9") {
		t.Errorf("stdout = %q, want the created comment fullname", stdout)
	}
}

func TestCommentCreate_BadParentIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "comment", "create", "--parent", "notafullname", "--text", "x")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "fullname") {
		t.Errorf("stderr = %q, want a fullname usage error", stderr)
	}
}

func TestPostEdit_UsesEditUserText(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"json":{"errors":[],"data":{"things":[{"kind":"t3","data":{"id":"abc","name":"t3_abc"}}]}}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "post", "edit", "t3_abc", "--text", "updated")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/editusertext" || got.Form.Get("thing_id") != "t3_abc" || got.Form.Get("text") != "updated" {
		t.Errorf("request = %s form=%v, want /api/editusertext thing_id=t3_abc text=updated", got.Path, got.Form)
	}
}

func TestPostDelete_FormShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "post", "delete", "t3_abc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/del" || got.Form.Get("id") != "t3_abc" {
		t.Errorf("request = %s form=%v, want /api/del id=t3_abc", got.Path, got.Form)
	}
}

func TestInboxList_FilterMapsToSegment(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"kind":"Listing","data":{"after":null,"children":[]}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "inbox", "list", "--filter", "unread")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/message/unread" {
		t.Errorf("path = %q, want /message/unread", got.Path)
	}
}

func TestInboxMarkRead_JoinsFullnames(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "inbox", "mark-read", "t4_a", "t1_b")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/read_message" || got.Form.Get("id") != "t4_a,t1_b" {
		t.Errorf("request = %s form=%v, want /api/read_message id=t4_a,t1_b", got.Path, got.Form)
	}
}

func TestMessageSend_ComposeForm(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"json":{"errors":[],"data":{}}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "message", "send", "--to", "alice", "--subject", "hi", "--text", "hello")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/api/compose" || got.Form.Get("to") != "alice" ||
		got.Form.Get("subject") != "hi" || got.Form.Get("text") != "hello" || got.Form.Get("api_type") != "json" {
		t.Errorf("request = %s form=%v, want /api/compose to=alice subject=hi text=hello api_type=json", got.Path, got.Form)
	}
}

func TestSubsList_StripsEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"kind":"Listing","data":{"after":null,"children":[{"kind":"t5","data":{"id":"s1","name":"t5_s1","display_name":"golang","subscribers":100}}]}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "subs", "list", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/subreddits/mine/subscriber" {
		t.Errorf("path = %q, want /subreddits/mine/subscriber", got.Path)
	}
	lines := decodeLines(t, stdout)
	if len(lines) != 1 || lines[0]["display_name"] != "golang" {
		t.Errorf("subs = %+v, want one flat subreddit display_name=golang", lines)
	}
}

func TestUserAbout_FlatIdentity(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"kind":"t2","data":{"id":"u1","name":"alice","link_karma":10,"comment_karma":20}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "user", "about", "alice", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Path != "/user/alice/about" {
		t.Errorf("path = %q, want /user/alice/about", got.Path)
	}
	var u map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &u); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout)
	}
	if u["name"] != "alice" || u["comment_karma"].(float64) != 20 {
		t.Errorf("user = %+v, want name=alice comment_karma=20", u)
	}
}
