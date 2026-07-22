package buffer

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAccountGetSelectsFieldsAndInjectsBearer(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"account":{"id":"acc1","email":"a@b.co","organizations":[{"id":"o1","name":"Org One"}]}}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "account", "get")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got.Method != http.MethodPost {
		t.Errorf("method=%s want POST", got.Method)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("auth=%q want Bearer tok-123", got.Auth)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type=%q", got.ContentType)
	}
	body := decodeReqBody(t, got.Body)
	if !strings.Contains(body.Query, "account {") || !strings.Contains(body.Query, "organizations") {
		t.Errorf("query missing account/organizations: %s", body.Query)
	}
	out := decodeOut(t, stdout)
	if out["id"] != "acc1" || out["email"] != "a@b.co" {
		t.Errorf("account output = %v", out)
	}
	orgs, ok := out["organizations"].([]any)
	if !ok || len(orgs) != 1 {
		t.Fatalf("organizations = %v", out["organizations"])
	}
}

func TestOrgListProjectsOrganizations(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"account":{"id":"acc1","email":"a@b.co","organizations":[{"id":"o1","name":"One"},{"id":"o2","name":"Two"}]}}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "org", "list")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	out := decodeOut(t, stdout)
	orgs, ok := out["organizations"].([]any)
	if !ok || len(orgs) != 2 {
		t.Fatalf("organizations = %v", out["organizations"])
	}
}

func TestChannelListRequiresOrgAndPassesVariable(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"channels":[{"id":"c1","name":"Brand X","service":"twitter"}]}}`, &got)
	defer srv.Close()

	// Missing --org is a usage error (exit 2).
	code, _, _ := run(t, srv, "channel", "list")
	if code != 2 {
		t.Fatalf("missing --org exit=%d want 2", code)
	}

	code, stdout, stderr := run(t, srv, "channel", "list", "--org", "o1")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	body := decodeReqBody(t, got.Body)
	input, _ := body.Variables["input"].(map[string]any)
	if input["organizationId"] != "o1" {
		t.Errorf("variables.input.organizationId = %v", input)
	}
	out := decodeOut(t, stdout)
	channels, ok := out["channels"].([]any)
	if !ok || len(channels) != 1 {
		t.Fatalf("channels = %v", out["channels"])
	}
}

func TestPostListIsOrgScopedWithOptionalFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"posts":{"pageInfo":{"hasNextPage":false,"endCursor":"cur"},"edges":[{"node":{"id":"p1","text":"hi","createdAt":"2026-01-01T00:00:00Z","channelId":"c1"}}]}}}`, &got)
	defer srv.Close()

	// Missing --org is a usage error.
	if code, _, _ := run(t, srv, "post", "list"); code != 2 {
		t.Fatalf("missing --org exit=%d want 2", code)
	}

	code, stdout, stderr := run(t, srv, "post", "list", "--org", "o1", "--channel", "c1", "--status", "sent", "--first", "20")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	body := decodeReqBody(t, got.Body)
	input, _ := body.Variables["input"].(map[string]any)
	if input["organizationId"] != "o1" {
		t.Errorf("organizationId = %v", input["organizationId"])
	}
	filter, _ := input["filter"].(map[string]any)
	chans, _ := filter["channelIds"].([]any)
	if len(chans) != 1 || chans[0] != "c1" {
		t.Errorf("filter.channelIds = %v", filter["channelIds"])
	}
	statuses, _ := filter["status"].([]any)
	if len(statuses) != 1 || statuses[0] != "sent" {
		t.Errorf("filter.status = %v (want [sent] array)", filter["status"])
	}
	if body.Variables["first"].(float64) != 20 {
		t.Errorf("first = %v", body.Variables["first"])
	}
	out := decodeOut(t, stdout)
	posts, ok := out["posts"].([]any)
	if !ok || len(posts) != 1 {
		t.Fatalf("posts = %v", out["posts"])
	}
	first := posts[0].(map[string]any)
	if first["id"] != "p1" || first["channelId"] != "c1" {
		t.Errorf("post node = %v", first)
	}
}

func TestPostCreateQueueDefaults(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"createPost":{"__typename":"PostActionSuccess","post":{"id":"p9","text":"hello","dueAt":null}}}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "post", "create", "--channel", "c1", "--text", "hello")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	body := decodeReqBody(t, got.Body)
	input, _ := body.Variables["input"].(map[string]any)
	if input["channelId"] != "c1" || input["text"] != "hello" {
		t.Errorf("input = %v", input)
	}
	if input["schedulingType"] != "automatic" {
		t.Errorf("schedulingType = %v want automatic", input["schedulingType"])
	}
	if input["mode"] != "addToQueue" {
		t.Errorf("mode = %v want addToQueue default", input["mode"])
	}
	if _, present := input["dueAt"]; present {
		t.Errorf("dueAt must be absent for queue mode: %v", input["dueAt"])
	}
	out := decodeOut(t, stdout)
	if out["id"] != "p9" || out["channelId"] != "c1" {
		t.Errorf("output = %v", out)
	}
}

func TestPostCreateCustomScheduledRequiresDueAt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"createPost":{"__typename":"PostActionSuccess","post":{"id":"p9","text":"hi","dueAt":"2026-03-10T15:00:00Z"}}}}`, &got)
	defer srv.Close()

	// customScheduled without --due-at is a usage error.
	if code, _, _ := run(t, srv, "post", "create", "--channel", "c1", "--text", "hi", "--mode", "customScheduled"); code != 2 {
		t.Fatalf("missing --due-at exit want 2")
	}

	code, _, stderr := run(t, srv, "post", "create", "--channel", "c1", "--text", "hi", "--mode", "customScheduled", "--due-at", "2026-03-10T15:00:00Z")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	body := decodeReqBody(t, got.Body)
	input, _ := body.Variables["input"].(map[string]any)
	if input["mode"] != "customScheduled" || input["dueAt"] != "2026-03-10T15:00:00Z" {
		t.Errorf("input = %v", input)
	}
}

func TestPostCreateBadModeIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "post", "create", "--channel", "c1", "--text", "x", "--mode", "bogus"); code != 2 {
		t.Fatalf("bad mode exit want 2")
	}
}

func TestPostCreateDraftAndRawJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"createPost":{"__typename":"PostActionSuccess","post":{"id":"p1","text":"t","dueAt":null}}}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "post", "create", "--channel", "c1", "--text", "t", "--draft",
		"--assets-json", `{"photos":["https://x/y.png"]}`, "--metadata-json", `{"twitter":{"foo":"bar"}}`)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	body := decodeReqBody(t, got.Body)
	input, _ := body.Variables["input"].(map[string]any)
	if input["saveToDraft"] != true {
		t.Errorf("saveToDraft = %v", input["saveToDraft"])
	}
	if _, ok := input["assets"].(map[string]any); !ok {
		t.Errorf("assets not attached: %v", input["assets"])
	}
	if _, ok := input["metadata"].(map[string]any); !ok {
		t.Errorf("metadata not attached: %v", input["metadata"])
	}
}

func TestPostCreateInvalidJSONIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "post", "create", "--channel", "c1", "--text", "t", "--assets-json", `{not json`); code != 2 {
		t.Fatalf("bad JSON exit want 2")
	}
}

func TestPostCreateVideoLinkAttachmentMutuallyExclusive(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()
	code, _, stderr := run(t, srv, "post", "create", "--channel", "c1", "--text", "t",
		"--assets-json", `{"videos":[{"id":"v1"}]}`,
		"--metadata-json", `{"facebook":{"linkAttachment":{"url":"https://x"}}}`)
	if code != 2 {
		t.Fatalf("mutual-exclusion exit=%d want 2", code)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("stderr = %q", stderr)
	}
	if len(got.Body) != 0 {
		t.Errorf("must not call API on usage error; body=%s", got.Body)
	}
}

func TestPostEditSendsRequiredFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"editPost":{"__typename":"PostActionSuccess","post":{"id":"p1","text":"new","dueAt":null}}}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "post", "edit", "--text", "x"); code != 2 {
		t.Fatalf("missing --id exit want 2")
	}

	code, stdout, stderr := run(t, srv, "post", "edit", "--id", "p1", "--text", "new")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	body := decodeReqBody(t, got.Body)
	input, _ := body.Variables["input"].(map[string]any)
	if input["id"] != "p1" || input["schedulingType"] != "automatic" || input["mode"] != "addToQueue" {
		t.Errorf("edit input = %v", input)
	}
	out := decodeOut(t, stdout)
	if out["id"] != "p1" || out["text"] != "new" {
		t.Errorf("edit output = %v", out)
	}
}

func TestPostDeleteReturnsID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"deletePost":{"__typename":"DeletePostSuccess","id":"p1"}}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "post", "delete"); code != 2 {
		t.Fatalf("missing --id exit want 2")
	}
	code, stdout, stderr := run(t, srv, "post", "delete", "--id", "p1")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if decodeOut(t, stdout)["id"] != "p1" {
		t.Errorf("delete output = %s", stdout)
	}
}

func TestIdeaCreateBuildsContent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"createIdea":{"__typename":"Idea","id":"i1","content":{"title":"T","text":"B"}}}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "idea", "create", "--text", "B"); code != 2 {
		t.Fatalf("missing --org exit want 2")
	}
	code, stdout, stderr := run(t, srv, "idea", "create", "--org", "o1", "--text", "B", "--title", "T")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	body := decodeReqBody(t, got.Body)
	input, _ := body.Variables["input"].(map[string]any)
	if input["organizationId"] != "o1" {
		t.Errorf("organizationId = %v", input["organizationId"])
	}
	content, _ := input["content"].(map[string]any)
	if content["text"] != "B" || content["title"] != "T" {
		t.Errorf("content = %v", content)
	}
	out := decodeOut(t, stdout)
	if out["id"] != "i1" || out["title"] != "T" || out["text"] != "B" {
		t.Errorf("idea output = %v", out)
	}
}

func TestMutationErrorArmSurfacesMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"createPost":{"__typename":"RestProxyError","message":"channel disconnected"}}}`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "post", "create", "--channel", "c1", "--text", "hi")
	if code != 1 {
		t.Fatalf("mutation error exit=%d want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on error: %q", stdout)
	}
	if !strings.Contains(stderr, "channel disconnected") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestUnknownTypenameStillFailsFast(t *testing.T) {
	var got capturedRequest
	// An error type outside the MutationError interface (no message field):
	// __typename != success must still be a failure.
	srv := newServer(t, 200, `{"data":{"createPost":{"__typename":"SomeFutureError"}}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "post", "create", "--channel", "c1", "--text", "hi")
	if code != 1 {
		t.Fatalf("unknown typename exit=%d want 1", code)
	}
	if !strings.Contains(stderr, "SomeFutureError") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestTopLevelGraphQLErrorsFailFast(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"errors":[{"message":"Field 'bogus' doesn't exist"}],"data":null}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "account", "get")
	if code != 1 {
		t.Fatalf("graphql errors exit=%d want 1", code)
	}
	if !strings.Contains(stderr, "doesn't exist") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"errors":[{"message":"Unauthorized"}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "account", "get")
	if result.ExitCode != 1 {
		t.Fatalf("exit=%d want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("401 should mark credential rejected; stderr=%s", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"createPost":{"__typename":"NotFoundError","message":"no such channel"}}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "--json", "post", "create", "--channel", "c1", "--text", "hi")
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || !strings.Contains(env.Error.Message, "no such channel") {
		t.Errorf("envelope = %+v", env.Error)
	}
}

func TestUsageErrorJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()
	code, _, stderr := run(t, srv, "--json", "channel", "list")
	if code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if !strings.Contains(stderr, `"kind":"usage"`) {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit=%d want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "BUFFER_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestUnknownSubcommandFails(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "post", "bogus"); code != 2 {
		t.Fatalf("unknown subcommand exit want 2")
	}
}
