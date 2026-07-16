package slides

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// recordedRequest is one request the fake Slides server saw.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

// route is a canned response for "METHOD /path".
type route struct {
	status int
	body   string
}

// fixture is a fake Slides API server: routes keyed by "METHOD /v1/...", every
// request recorded in order. Retry backoff sleeps are recorded instead of
// slept so tests stay fast. genID, when set, is injected so batchUpdate bodies
// are deterministic.
type fixture struct {
	srv      *httptest.Server
	routes   map[string]route
	requests []recordedRequest
	sleeps   []time.Duration
	genID    func(prefix string) string
}

func newFixture(t *testing.T, routes map[string]route) *fixture {
	t.Helper()
	f := &fixture{routes: routes}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		f.requests = append(f.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body.Bytes(),
		})
		rt, ok := f.routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"no route"}}`))
			return
		}
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) setRoute(key string, rt route) {
	f.routes[key] = rt
}

// last returns the most recent request matching method+path.
func (f *fixture) last(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method == method && f.requests[i].Path == path {
			return f.requests[i]
		}
	}
	t.Fatalf("no recorded request %s %s", method, path)
	return recordedRequest{}
}

func (f *fixture) newService(out, errBuf *bytes.Buffer) *Service {
	return &Service{
		BaseURL: f.srv.URL + "/v1",
		HC:      f.srv.Client(),
		Out:     out,
		Err:     errBuf,
		sleep:   func(d time.Duration) { f.sleeps = append(f.sleeps, d) },
		genID:   f.genID,
	}
}

func (f *fixture) run(t *testing.T, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := f.newService(&out, &errBuf)
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: "ya29.test-token"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func (f *fixture) runOK(t *testing.T, args ...string) string {
	t.Helper()
	result, stdout, stderr := f.run(t, args...)
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", result.ExitCode, stderr)
	}
	return stdout
}

// decodeBody unmarshals a recorded JSON request body into a generic map.
func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode request body %q: %v", raw, err)
	}
	return m
}

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"presentations", "get", "pid"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "SLIDES_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

const outlineDeck = `{
  "presentationId": "pid123",
  "title": "Q3 Review",
  "layouts": [
    {"objectId": "layout1", "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Title and body"}}
  ],
  "slides": [
    {
      "objectId": "slide1",
      "slideProperties": {
        "layoutObjectId": "layout1",
        "notesPage": {
          "notesProperties": {"speakerNotesObjectId": "notes1"},
          "pageElements": [
            {"objectId": "notes1", "shape": {"text": {"textElements": [{"textRun": {"content": "Remember the numbers\n"}}]}}}
          ]
        }
      },
      "pageElements": [
        {"objectId": "title1", "shape": {"shapeType": "TEXT_BOX", "placeholder": {"type": "TITLE"}, "text": {"textElements": [{"textRun": {"content": "Hello World\n"}}]}}},
        {"objectId": "body1", "shape": {"placeholder": {"type": "BODY"}, "text": {"textElements": [{"textRun": {"content": "point one\npoint two\n"}}]}}}
      ]
    }
  ]
}`

func TestPresentationsGet_OutlineAndJSON(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/presentations/pid123": {http.StatusOK, outlineDeck},
	})
	stdout := f.runOK(t, "presentations", "get", "https://docs.google.com/presentation/d/pid123/edit")
	for _, want := range []string{
		"Presentation: Q3 Review (pid123)",
		"slide=slide1",
		"layout=TITLE_AND_BODY",
		"title1 [TITLE]: Hello World",
		"body1 [BODY]: point one point two",
		"notes: Remember the numbers",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("outline missing %q\n---\n%s", want, stdout)
		}
	}
	got := f.last(t, "GET", "/v1/presentations/pid123")
	if got.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the bearer token", got.Auth)
	}

	stdout = f.runOK(t, "presentations", "get", "pid123", "--json")
	var probe map[string]any
	if err := json.Unmarshal([]byte(stdout), &probe); err != nil {
		t.Fatalf("--json output is not valid JSON: %v", err)
	}
	if probe["presentationId"] != "pid123" {
		t.Errorf("--json output missing raw presentationId: %v", probe["presentationId"])
	}
}

func TestPresentationsGet_SlideFilter(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/presentations/pid123": {http.StatusOK, outlineDeck},
	})
	stdout := f.runOK(t, "presentations", "get", "pid123", "--slide", "slide1")
	if !strings.Contains(stdout, "slide=slide1") {
		t.Errorf("filtered outline missing slide1: %s", stdout)
	}
	stdout = f.runOK(t, "presentations", "get", "pid123", "--slide", "2")
	if strings.Contains(stdout, "slide=slide1") {
		t.Errorf("--slide 2 should not print slide1: %s", stdout)
	}
}

func TestPresentationsCreate(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations": {http.StatusOK, `{"presentationId":"newpid","title":"Deck"}`},
	})
	stdout := f.runOK(t, "presentations", "create", "--title", "Deck")
	if !strings.Contains(stdout, "newpid") || !strings.Contains(stdout, "docs.google.com/presentation/d/newpid") {
		t.Errorf("create output = %q, want id + URL", stdout)
	}
	got := f.last(t, "POST", "/v1/presentations")
	if decodeBody(t, got.Body)["title"] != "Deck" {
		t.Errorf("create body = %s, want title=Deck", got.Body)
	}
}

func TestPresentationsCreate_RequiresTitle(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "presentations", "create")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "--title is required") {
		t.Errorf("stderr = %q, want the required-title error", stderr)
	}
	if len(f.requests) != 0 {
		t.Errorf("validation failure must not reach the API; saw %d requests", len(f.requests))
	}
}

func TestSlidesAdd_BatchBody(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{"createSlide":{"objectId":"s_ID"}}]}`},
	})
	f.genID = func(prefix string) string { return prefix + "ID" }
	stdout := f.runOK(t, "slides", "add", "pid123", "--layout", "TITLE_AND_BODY", "--title", "Agenda", "--body", "First\nSecond", "--at", "2")
	if !strings.Contains(stdout, "added slide s_ID") {
		t.Errorf("stdout = %q, want the created slide id", stdout)
	}
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	body := decodeBody(t, got.Body)
	requests, ok := body["requests"].([]any)
	if !ok || len(requests) != 3 {
		t.Fatalf("want 3 requests (createSlide + 2 insertText), got %v", body["requests"])
	}
	create := requests[0].(map[string]any)["createSlide"].(map[string]any)
	if create["objectId"] != "s_ID" {
		t.Errorf("createSlide objectId = %v, want s_ID", create["objectId"])
	}
	if create["insertionIndex"].(float64) != 2 {
		t.Errorf("insertionIndex = %v, want 2", create["insertionIndex"])
	}
	if create["slideLayoutReference"].(map[string]any)["predefinedLayout"] != "TITLE_AND_BODY" {
		t.Errorf("layout = %v, want TITLE_AND_BODY", create["slideLayoutReference"])
	}
	mappings := create["placeholderIdMappings"].([]any)
	if len(mappings) != 2 {
		t.Fatalf("want 2 placeholder mappings, got %d", len(mappings))
	}
	// insertText requests carry the assigned placeholder ids and the content.
	rawBody := string(got.Body)
	for _, want := range []string{`"objectId":"t_ID"`, `"text":"Agenda"`, `"objectId":"b_ID"`, `"text":"First\nSecond"`, `"type":"TITLE"`, `"type":"BODY"`} {
		if !strings.Contains(rawBody, want) {
			t.Errorf("batchUpdate body missing %q\n%s", want, rawBody)
		}
	}
}

func TestSlidesAdd_NoPlaceholders(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{"createSlide":{"objectId":"s_ID"}}]}`},
	})
	f.genID = func(prefix string) string { return prefix + "ID" }
	f.runOK(t, "slides", "add", "pid123", "--layout", "BLANK")
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	body := decodeBody(t, got.Body)
	requests := body["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("BLANK slide with no title/body should be one request, got %d", len(requests))
	}
	create := requests[0].(map[string]any)["createSlide"].(map[string]any)
	if _, ok := create["insertionIndex"]; ok {
		t.Error("insertionIndex should be absent when --at is not passed")
	}
	if _, ok := create["placeholderIdMappings"]; ok {
		t.Error("placeholderIdMappings should be absent with no title/body")
	}
}

func TestSlidesDuplicate_WithMove(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{"duplicateObject":{"objectId":"s_ID"}}]}`},
	})
	f.genID = func(prefix string) string { return prefix + "ID" }
	f.runOK(t, "slides", "duplicate", "pid123", "slideX", "--at", "0")
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	requests := decodeBody(t, got.Body)["requests"].([]any)
	if len(requests) != 2 {
		t.Fatalf("duplicate --at should emit duplicateObject + updateSlidesPosition, got %d", len(requests))
	}
	dup := requests[0].(map[string]any)["duplicateObject"].(map[string]any)
	if dup["objectId"] != "slideX" {
		t.Errorf("duplicate source = %v, want slideX", dup["objectId"])
	}
	if dup["objectIds"].(map[string]any)["slideX"] != "s_ID" {
		t.Errorf("duplicate id mapping = %v, want slideX->s_ID", dup["objectIds"])
	}
	pos := requests[1].(map[string]any)["updateSlidesPosition"].(map[string]any)
	if pos["slideObjectIds"].([]any)[0] != "s_ID" || pos["insertionIndex"].(float64) != 0 {
		t.Errorf("updateSlidesPosition = %v, want [s_ID] at 0", pos)
	}
}

func TestSlidesDelete_MultiIDs(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{},{}]}`},
	})
	stdout := f.runOK(t, "slides", "delete", "pid123", "slideA slideB")
	if !strings.Contains(stdout, "deleted 2 slide(s)") {
		t.Errorf("stdout = %q, want two deletions", stdout)
	}
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	requests := decodeBody(t, got.Body)["requests"].([]any)
	if len(requests) != 2 {
		t.Fatalf("whitespace-joined ids should split to 2 deleteObject requests, got %d", len(requests))
	}
	if requests[0].(map[string]any)["deleteObject"].(map[string]any)["objectId"] != "slideA" {
		t.Errorf("first delete = %v, want slideA", requests[0])
	}
}

func TestTextReplace_PageScopeAndCount(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{"replaceAllText":{"occurrencesChanged":3}}]}`},
	})
	stdout := f.runOK(t, "text", "replace", "pid123", "--find", "{{name}}", "--replace", "Ada", "--match-case", "--slide", "slide1", "--slide", "slide2")
	if !strings.Contains(stdout, "replaced 3 occurrence(s)") {
		t.Errorf("stdout = %q, want the changed count", stdout)
	}
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	replace := decodeBody(t, got.Body)["requests"].([]any)[0].(map[string]any)["replaceAllText"].(map[string]any)
	contains := replace["containsText"].(map[string]any)
	if contains["text"] != "{{name}}" || contains["matchCase"] != true {
		t.Errorf("containsText = %v, want the find text + matchCase", contains)
	}
	pages := replace["pageObjectIds"].([]any)
	if len(pages) != 2 || pages[0] != "slide1" {
		t.Errorf("pageObjectIds = %v, want [slide1 slide2]", pages)
	}
}

func TestTextInsert_Append(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /v1/presentations/pid123":              {http.StatusOK, outlineDeck},
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{}]}`},
	})
	f.runOK(t, "text", "insert", "pid123", "--object", "body1", "--text", " more", "--append")
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	insert := decodeBody(t, got.Body)["requests"].([]any)[0].(map[string]any)["insertText"].(map[string]any)
	// "point one\npoint two\n" is 20 UTF-16 units; append lands just before the
	// trailing newline at index 19.
	if insert["insertionIndex"].(float64) != 19 {
		t.Errorf("append insertionIndex = %v, want 19", insert["insertionIndex"])
	}
}

func TestTextDelete_RangeVariants(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{}]}`},
	})
	f.runOK(t, "text", "delete", "pid123", "--object", "body1", "--range", "2:5")
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	rng := decodeBody(t, got.Body)["requests"].([]any)[0].(map[string]any)["deleteText"].(map[string]any)["textRange"].(map[string]any)
	if rng["type"] != "FIXED_RANGE" || rng["startIndex"].(float64) != 2 || rng["endIndex"].(float64) != 5 {
		t.Errorf("textRange = %v, want FIXED_RANGE 2..5", rng)
	}
}

func TestImagesInsert_SizeAndPosition(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[{"createImage":{"objectId":"img_ID"}}]}`},
	})
	f.genID = func(prefix string) string { return prefix + "ID" }
	f.runOK(t, "images", "insert", "pid123", "--slide", "slide1", "--url", "https://example.com/a.png", "--at", "100,50", "--size", "400x300")
	got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
	img := decodeBody(t, got.Body)["requests"].([]any)[0].(map[string]any)["createImage"].(map[string]any)
	if img["url"] != "https://example.com/a.png" || img["objectId"] != "img_ID" {
		t.Errorf("createImage = %v, want url + generated id", img)
	}
	props := img["elementProperties"].(map[string]any)
	if props["pageObjectId"] != "slide1" {
		t.Errorf("pageObjectId = %v, want slide1", props["pageObjectId"])
	}
	size := props["size"].(map[string]any)
	if size["width"].(map[string]any)["magnitude"].(float64) != 400 || size["width"].(map[string]any)["unit"] != "PT" {
		t.Errorf("size width = %v, want 400 PT", size["width"])
	}
	tf := props["transform"].(map[string]any)
	if tf["translateX"].(float64) != 100 || tf["translateY"].(float64) != 50 || tf["unit"] != "PT" {
		t.Errorf("transform = %v, want translate 100,50 PT", tf)
	}
}

func TestBatchUpdate_Passthrough(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantReqs int
	}{
		{"bare array", `[{"deleteObject":{"objectId":"a"}},{"deleteObject":{"objectId":"b"}}]`, 2},
		{"single object", `{"deleteObject":{"objectId":"a"}}`, 1},
		{"full body", `{"requests":[{"deleteObject":{"objectId":"a"}}]}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"POST /v1/presentations/pid123:batchUpdate": {http.StatusOK, `{"presentationId":"pid123","replies":[]}`},
			})
			f.runOK(t, "batch-update", "pid123", "--requests", tc.input)
			got := f.last(t, "POST", "/v1/presentations/pid123:batchUpdate")
			requests := decodeBody(t, got.Body)["requests"].([]any)
			if len(requests) != tc.wantReqs {
				t.Errorf("normalized requests = %d, want %d (%s)", len(requests), tc.wantReqs, got.Body)
			}
		})
	}
}

func TestBatchUpdate_RequiresOneSource(t *testing.T) {
	f := newFixture(t, map[string]route{})
	result, _, stderr := f.run(t, "batch-update", "pid123")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "exactly one of --requests or --requests-file") {
		t.Errorf("stderr = %q, want the one-source error", stderr)
	}
}

func TestBatchUpdate_AtomicFailurePassthrough(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusBadRequest, `{"error":{"status":"INVALID_ARGUMENT","message":"Invalid requests[1]: bad objectId"}}`},
	})
	result, _, stderr := f.run(t, "batch-update", "pid123", "--requests", `[{"deleteObject":{"objectId":"a"}}]`)
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "Invalid requests[1]") {
		t.Errorf("stderr = %q, want the request-index error surfaced verbatim", stderr)
	}
}

func TestPagesThumbnail_Download(t *testing.T) {
	f := newFixture(t, map[string]route{})
	f.setRoute("GET /v1/presentations/pid123/pages/slide1/thumbnail",
		route{http.StatusOK, `{"contentUrl":"` + f.srv.URL + `/thumb.png","width":1600,"height":900}`})
	f.setRoute("GET /thumb.png", route{http.StatusOK, "PNGBYTES"})
	dir := t.TempDir()
	stdout := f.runOK(t, "pages", "thumbnail", "pid123", "slide1", "--save", dir, "--size", "LARGE")
	if !strings.Contains(stdout, "1600x900") {
		t.Errorf("stdout = %q, want dimensions", stdout)
	}
	got := f.last(t, "GET", "/v1/presentations/pid123/pages/slide1/thumbnail")
	if !strings.Contains(got.Query, "thumbnailProperties.thumbnailSize=LARGE") {
		t.Errorf("query = %q, want the size param", got.Query)
	}
	data, err := os.ReadFile(filepath.Join(dir, "slide1.png"))
	if err != nil {
		t.Fatalf("thumbnail not written: %v", err)
	}
	if string(data) != "PNGBYTES" {
		t.Errorf("thumbnail bytes = %q, want PNGBYTES", data)
	}
}

func TestScopeHintOn403(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/presentations/pid123:batchUpdate": {http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED","message":"Request had insufficient authentication scopes"}}`},
	})
	result, _, stderr := f.run(t, "batch-update", "pid123", "--requests", `[{"createSheetsChart":{}}]`)
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "insufficient authentication scopes") {
		t.Errorf("stderr = %q, want the provider message", stderr)
	}
	if !strings.Contains(stderr, "possibly missing scope — reconnect and grant access") {
		t.Errorf("stderr = %q, want the reconnect hint on 403", stderr)
	}
	if result.CredentialRejected {
		t.Error("403 PERMISSION_DENIED must not reject the credential")
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		providerStatus string
		wantRejected   bool
	}{
		{"HTTP unauthorized", http.StatusUnauthorized, "UNAUTHENTICATED", true},
		{"explicit unauthenticated status", http.StatusBadRequest, "UNAUTHENTICATED", true},
		{"permission denied", http.StatusForbidden, "PERMISSION_DENIED", false},
		{"not found", http.StatusNotFound, "NOT_FOUND", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]route{
				"GET /v1/presentations/pid123": {tc.status, `{"error":{"status":"` + tc.providerStatus + `","message":"provider message"}}`},
			})
			result, _, _ := f.run(t, "presentations", "get", "pid123")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestArgvParsing_Failures(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"presentations", "explode"}, "explode"},
		{"get without id", []string{"presentations", "get"}, "accepts 1 arg"},
		{"text insert without object", []string{"text", "insert", "pid", "--text", "x"}, "--object is required"},
		{"text insert at and append", []string{"text", "insert", "pid", "--object", "o", "--text", "x", "--at", "1", "--append"}, "none of the others can be"},
		{"move without to", []string{"slides", "move", "pid", "slideA"}, "--to is required"},
		{"bad range", []string{"text", "delete", "pid", "--object", "o", "--range", "nope"}, "--range"},
		{"bad thumbnail size", []string{"pages", "thumbnail", "pid", "p1", "--size", "HUGE"}, "--size must be LARGE"},
	}
	f := newFixture(t, map[string]route{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, _, stderr := f.run(t, tc.args...)
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1 (stderr: %s)", result.ExitCode, stderr)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("argv failures must not reach the API; saw %d requests", len(f.requests))
	}
}
