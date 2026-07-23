package mastodon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// harness spins up an httptest server and runs one mastodon invocation through
// the real Execute entry point, with the combined credential pointing the
// instance base URL at the fake server. It captures stdout, stderr, and the
// execution result so tests assert on request shape and rendered output.
type harness struct {
	t       *testing.T
	server  *httptest.Server
	last    *recordedRequest
	handler func(w http.ResponseWriter, r *recordedRequest)
}

type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t}
	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec := &recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Header: r.Header.Clone(),
			Body:   body,
		}
		h.last = rec
		if h.handler != nil {
			h.handler(w, rec)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(h.server.Close)
	return h
}

// run executes args with the combined credential "<serverURL> testtoken".
func (h *harness) run(args ...string) (stdout, stderr string, exit int) {
	return h.runWithToken("testtoken", args...)
}

func (h *harness) runWithToken(token string, args ...string) (stdout, stderr string, exit int) {
	h.t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf, HC: h.server.Client()}
	env := map[string]string{EnvAccessToken: h.server.URL + " " + token}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		h.t.Fatalf("Execute returned error: %v", err)
	}
	return out.String(), errBuf.String(), res.ExitCode
}

func TestWhoamiInjectsBearerAndDerivesBaseURL(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"42","username":"alice","acct":"alice","display_name":"Alice","url":"https://m.example/@alice","note":"<p>Bio &amp; more</p>","followers_count":10,"following_count":5,"statuses_count":3}`))
	}
	stdout, stderr, exit := h.run("whoami")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if h.last.Method != http.MethodGet || h.last.Path != "/api/v1/accounts/verify_credentials" {
		t.Fatalf("request = %s %s, want GET /api/v1/accounts/verify_credentials", h.last.Method, h.last.Path)
	}
	if got := h.last.Header.Get("Authorization"); got != "Bearer testtoken" {
		t.Fatalf("Authorization = %q, want Bearer testtoken", got)
	}
	var detail accountDetail
	if err := json.Unmarshal([]byte(stdout), &detail); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, stdout)
	}
	if detail.Acct != "alice" || detail.NoteText != "Bio & more" {
		t.Fatalf("detail = %+v, want acct alice and stripped bio", detail)
	}
}

func TestCredentialWithoutSpaceIsUsageFailure(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"whoami"}, map[string]string{EnvAccessToken: "https://mastodon.social"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "joined by a space") {
		t.Fatalf("stderr = %q, want guidance about the space-joined credential", errBuf.String())
	}
}

func TestMissingCredentialJSONError(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, _ := svc.Execute(context.Background(), []string{"whoami", "--json"}, map[string]string{})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr not a JSON error envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Code != "usage_error" {
		t.Fatalf("error code = %q, want usage_error", env.Error.Code)
	}
}

func TestPostCreateSendsIdempotencyKeyAndBody(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"111","url":"https://m.example/@alice/111","visibility":"unlisted","created_at":"2026-07-22T00:00:00Z"}`))
	}
	stdout, stderr, exit := h.run("post", "create", "--text", "hello world", "--reply-to", "99", "--cw", "spoiler", "--visibility", "unlisted")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if h.last.Method != http.MethodPost || h.last.Path != "/api/v1/statuses" {
		t.Fatalf("request = %s %s, want POST /api/v1/statuses", h.last.Method, h.last.Path)
	}
	key := h.last.Header.Get("Idempotency-Key")
	if key == "" {
		t.Fatal("Idempotency-Key header missing on post create")
	}
	var body statusCreateRequest
	if err := json.Unmarshal(h.last.Body, &body); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if body.Status != "hello world" || body.InReplyToID != "99" || body.SpoilerText != "spoiler" || body.Visibility != "unlisted" {
		t.Fatalf("body = %+v, want mapped fields", body)
	}
	// Determinism: the same parameters derive the same key.
	if key2 := idempotencyKey(body); key2 != key {
		t.Fatalf("idempotency key not deterministic: %q vs %q", key, key2)
	}
	var created postCreated
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, stdout)
	}
	if created.ID != "111" || created.Visibility != "unlisted" {
		t.Fatalf("created = %+v", created)
	}
}

func TestPostCreateRejectsBadVisibility(t *testing.T) {
	h := newHarness(t)
	_, stderr, exit := h.run("post", "create", "--text", "hi", "--visibility", "nope")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
	if !strings.Contains(stderr, "invalid --visibility") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestPostCreateRequiresTextOrImage(t *testing.T) {
	h := newHarness(t)
	_, _, exit := h.run("post", "create")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
}

func TestPostCreateWithMediaUploadsPollsThenAttaches(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(imgPath, []byte("PNGDATA"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := newHarness(t)
	var uploadedDescription string
	var pollCount int
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		switch {
		case r.Method == http.MethodPost && r.Path == "/api/v2/media":
			uploadedDescription = multipartField(r, "description")
			w.WriteHeader(http.StatusAccepted) // 202 processing, no url yet
			_, _ = w.Write([]byte(`{"id":"media-1","url":null}`))
		case r.Method == http.MethodGet && r.Path == "/api/v1/media/media-1":
			pollCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"media-1","url":"https://m.example/media/1.png"}`))
		case r.Method == http.MethodPost && r.Path == "/api/v1/statuses":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"222","url":"https://m.example/@alice/222","visibility":"public","created_at":"2026-07-22T00:00:00Z"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.Path)
		}
	}
	stdout, stderr, exit := h.run("post", "create", "--text", "with pic", "--image", imgPath, "--alt", "a picture")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if uploadedDescription != "a picture" {
		t.Fatalf("uploaded description = %q, want the --alt text", uploadedDescription)
	}
	if pollCount < 1 {
		t.Fatal("expected at least one media poll after 202")
	}
	// The final status body must carry the resolved media id.
	var body statusCreateRequest
	_ = json.Unmarshal(h.last.Body, &body)
	if len(body.MediaIDs) != 1 || body.MediaIDs[0] != "media-1" {
		t.Fatalf("media_ids = %v, want [media-1]", body.MediaIDs)
	}
	if !strings.Contains(stdout, "222") {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestTimelineHomeParsesLinkCursor(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.Header().Set("Link", `<https://m.example/api/v1/timelines/home?max_id=88>; rel="next", <https://m.example/api/v1/timelines/home?min_id=90>; rel="prev"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"90","url":"https://m.example/@a/90","content":"<p>Hi <b>there</b></p>","created_at":"2026-07-22T00:00:00Z","replies_count":1,"reblogs_count":2,"favourites_count":3,"account":{"id":"1","acct":"a","display_name":"A"}}]`))
	}
	stdout, stderr, exit := h.run("timeline", "home", "--limit", "5", "--cursor", "100")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got := h.last.Query.Get("limit"); got != "5" {
		t.Fatalf("limit query = %q, want 5", got)
	}
	if got := h.last.Query.Get("max_id"); got != "100" {
		t.Fatalf("max_id query = %q, want 100 (from --cursor)", got)
	}
	var page timelinePage
	if err := json.Unmarshal([]byte(stdout), &page); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, stdout)
	}
	if page.Cursor != "88" {
		t.Fatalf("cursor = %q, want 88 from Link header next", page.Cursor)
	}
	if len(page.Posts) != 1 || page.Posts[0].ContentText != "Hi there" {
		t.Fatalf("posts = %+v, want stripped content 'Hi there'", page.Posts)
	}
}

func TestAccountGetResolvesHandleThenFetches(t *testing.T) {
	h := newHarness(t)
	var lookupSeen, getSeen bool
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		switch r.Path {
		case "/api/v1/accounts/lookup":
			lookupSeen = true
			if got := r.Query.Get("acct"); got != "bob@other.social" {
				t.Errorf("lookup acct = %q, want bob@other.social", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"555","username":"bob","acct":"bob@other.social"}`))
		case "/api/v1/accounts/555":
			getSeen = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"555","username":"bob","acct":"bob@other.social","display_name":"Bob"}`))
		default:
			t.Errorf("unexpected path %s", r.Path)
		}
	}
	stdout, stderr, exit := h.run("account", "get", "@bob@other.social")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if !lookupSeen || !getSeen {
		t.Fatalf("lookup=%v get=%v, want both", lookupSeen, getSeen)
	}
	if !strings.Contains(stdout, `"id":"555"`) {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestAccountGetNumericIdSkipsLookup(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		if r.Path == "/api/v1/accounts/lookup" {
			t.Error("numeric id must not trigger lookup")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"777","acct":"c"}`))
	}
	_, stderr, exit := h.run("account", "get", "777")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if h.last.Path != "/api/v1/accounts/777" {
		t.Fatalf("path = %s, want /api/v1/accounts/777", h.last.Path)
	}
}

func TestSearchRequiresQueryAndUsesV2(t *testing.T) {
	h := newHarness(t)
	_, _, exit := h.run("search")
	if exit != 2 {
		t.Fatalf("missing --q exit = %d, want 2", exit)
	}

	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accounts":[{"id":"1","acct":"a"}],"statuses":[],"hashtags":[{"name":"golang"}]}`))
	}
	stdout, stderr, exit := h.run("search", "--q", "golang", "--type", "hashtags")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if h.last.Path != "/api/v2/search" || h.last.Query.Get("q") != "golang" || h.last.Query.Get("type") != "hashtags" {
		t.Fatalf("request path=%s q=%s type=%s", h.last.Path, h.last.Query.Get("q"), h.last.Query.Get("type"))
	}
	if !strings.Contains(stdout, "golang") {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestApiRejectsAuthorizationOverride(t *testing.T) {
	h := newHarness(t)
	_, stderr, exit := h.run("api", "GET", "/api/v1/bookmarks", "--header", "Authorization: Bearer evil")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
	if !strings.Contains(stderr, "cannot be overridden") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestApiForwardsMethodPathQueryAndBody(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
	stdout, stderr, exit := h.run("api", "POST", "/api/v1/lists", "--query", "foo=bar", "--body", `{"title":"t"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if h.last.Method != http.MethodPost || h.last.Path != "/api/v1/lists" {
		t.Fatalf("request = %s %s", h.last.Method, h.last.Path)
	}
	if h.last.Query.Get("foo") != "bar" {
		t.Fatalf("query foo = %q", h.last.Query.Get("foo"))
	}
	if string(h.last.Body) != `{"title":"t"}` {
		t.Fatalf("body = %s", h.last.Body)
	}
	if !strings.Contains(stdout, "ok") {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestAPIErrorEnvelopeAndExitCode(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"Validation failed: Text can't be blank"}`))
	}
	_, stderr, exit := h.run("post", "create", "--text", "x", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (api error)", exit)
	}
	var env struct {
		Error struct {
			Code          string `json:"code"`
			Status        int    `json:"status"`
			ProviderError string `json:"provider_error"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Code != "api_error" || env.Error.Status != 422 {
		t.Fatalf("envelope = %+v, want api_error/422", env.Error)
	}
	if !strings.Contains(env.Error.ProviderError, "Validation failed") {
		t.Fatalf("provider_error = %q", env.Error.ProviderError)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"The access token is invalid"}`))
	}
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf, HC: h.server.Client()}
	env := map[string]string{EnvAccessToken: h.server.URL + " badtoken"}
	res, _ := svc.Execute(context.Background(), []string{"whoami"}, env)
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Fatal("401 must set CredentialRejected so the engine invalidates the token")
	}
}

func TestForbiddenDoesNotRejectCredential(t *testing.T) {
	h := newHarness(t)
	h.handler = func(w http.ResponseWriter, r *recordedRequest) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"This action is not allowed"}`))
	}
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf, HC: h.server.Client()}
	env := map[string]string{EnvAccessToken: h.server.URL + " tok"}
	res, _ := svc.Execute(context.Background(), []string{"favourite", "--id", "1"}, env)
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Fatal("403 is a scope problem, not a bad token — must NOT reject the credential")
	}
}

func TestHTMLToText(t *testing.T) {
	cases := map[string]string{
		"<p>line one</p><p>line two</p>": "line one\nline two",
		"a<br>b":                         "a\nb",
		"plain":                          "plain",
		"&lt;tag&gt; &amp; more":         "<tag> & more",
		`<p>hi <a href="x">link</a></p>`: "hi link",
	}
	for in, want := range cases {
		if got := htmlToText(in); got != want {
			t.Errorf("htmlToText(%q) = %q, want %q", in, got, want)
		}
	}
}

// multipartField parses a captured multipart body and returns the value of the
// named plain field (used to assert the media --alt description).
func multipartField(r *recordedRequest, name string) string {
	ct := r.Header.Get("Content-Type")
	const marker = "boundary="
	i := strings.Index(ct, marker)
	if i < 0 {
		return ""
	}
	boundary := ct[i+len(marker):]
	reader := multipart.NewReader(bytes.NewReader(r.Body), boundary)
	for {
		part, err := reader.NextPart()
		if err != nil {
			return ""
		}
		if part.FormName() == name {
			b, _ := io.ReadAll(part)
			return string(b)
		}
	}
}
