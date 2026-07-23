package hootsuite

import (
	"net/http"
	"strings"
	"testing"
)

func TestMeUnwrapsEnvelopeAndSendsBearer(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"12345","email":"a@b.com","fullName":"Ann"}}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "me")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/me" {
		t.Errorf("request = %s %s, want GET /me", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	// Envelope unwrapped: the inner object is printed, not the {"data":...} wrapper.
	out := decodeJSON(t, stdout).(map[string]any)
	if out["id"] != "12345" || out["email"] != "a@b.com" {
		t.Errorf("unwrapped output = %v, want inner member object", out)
	}
	if _, wrapped := out["data"]; wrapped {
		t.Errorf("output still wrapped in data envelope: %s", stdout)
	}
}

func TestOrgList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"1"}]}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "org", "list")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/me/organizations" {
		t.Errorf("path = %s, want /me/organizations", got.Path)
	}
	if _, ok := decodeJSON(t, stdout).([]any); !ok {
		t.Errorf("output not a JSON array: %s", stdout)
	}
}

func TestProfileListArrayUnwrap(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[{"id":"118264228","type":"TWITTER"}]}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "profile", "list")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/socialProfiles" {
		t.Errorf("path = %s, want /socialProfiles", got.Path)
	}
	arr, ok := decodeJSON(t, stdout).([]any)
	if !ok || len(arr) != 1 {
		t.Errorf("output not a single-element array: %s", stdout)
	}
}

func TestProfileGetAndTeams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"42"}}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "profile", "get", "42"); exit != 0 {
		t.Fatalf("get exit = %d", exit)
	}
	if got.Path != "/socialProfiles/42" {
		t.Errorf("get path = %s, want /socialProfiles/42", got.Path)
	}

	if exit, _, _ := run(t, srv, "profile", "teams", "42"); exit != 0 {
		t.Fatalf("teams exit = %d", exit)
	}
	if got.Path != "/socialProfiles/42/teams" {
		t.Errorf("teams path = %s, want /socialProfiles/42/teams", got.Path)
	}
}

func TestMessageScheduleBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"m1","state":"SCHEDULED"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"message", "schedule",
		"--text", "hello world",
		"--profile", "118264228",
		"--profile", "220001111",
		"--send-time", "2029-03-01T14:00:00Z",
		"--tag", "launch",
		"--email-notification",
		"--media-id", "abc123",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/messages" {
		t.Errorf("request = %s %s, want POST /messages", got.Method, got.Path)
	}
	if got.CType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.CType)
	}
	body := decodeBody(t, got.Body)
	if body["text"] != "hello world" {
		t.Errorf("text = %v", body["text"])
	}
	ids, ok := body["socialProfileIds"].([]any)
	if !ok || len(ids) != 2 {
		t.Fatalf("socialProfileIds = %v, want 2 numeric ids", body["socialProfileIds"])
	}
	// IDs must be JSON numbers, not strings.
	if _, isNum := ids[0].(float64); !isNum {
		t.Errorf("socialProfileIds[0] = %T, want number", ids[0])
	}
	if body["scheduledSendTime"] != "2029-03-01T14:00:00Z" {
		t.Errorf("scheduledSendTime = %v", body["scheduledSendTime"])
	}
	if body["emailNotification"] != true {
		t.Errorf("emailNotification = %v, want true", body["emailNotification"])
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "launch" {
		t.Errorf("tags = %v", body["tags"])
	}
	media, ok := body["media"].([]any)
	if !ok || len(media) != 1 {
		t.Fatalf("media = %v", body["media"])
	}
	if media[0].(map[string]any)["id"] != "abc123" {
		t.Errorf("media[0].id = %v", media[0])
	}
}

func TestMessageScheduleOmitsSendTimeWhenAbsent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"m1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "message", "schedule", "--text", "now", "--profile", "1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	body := decodeBody(t, got.Body)
	if _, present := body["scheduledSendTime"]; present {
		t.Errorf("scheduledSendTime should be omitted for soonest-possible send: %s", got.Body)
	}
}

func TestMessageScheduleRejectsNonZSendTime(t *testing.T) {
	// No server call should be made; usage error before the POST.
	exit, _, stderr := run(t, nil, "message", "schedule",
		"--text", "x", "--profile", "1",
		"--send-time", "2029-03-01T14:00:00+02:00")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage); stderr = %s", exit, stderr)
	}
}

func TestMessageScheduleRejectsNonNumericProfile(t *testing.T) {
	exit, _, _ := run(t, nil, "message", "schedule", "--text", "x", "--profile", "notanumber")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
}

func TestMessageSchedulePinterestExtendedInfo(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"m1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "message", "schedule",
		"--text", "pin it",
		"--profile", "999",
		"--board-id", "12345678909876",
		"--destination-url", "https://example.com",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	body := decodeBody(t, got.Body)
	ext, ok := body["extendedInfo"].([]any)
	if !ok || len(ext) != 1 {
		t.Fatalf("extendedInfo = %v", body["extendedInfo"])
	}
	entry := ext[0].(map[string]any)
	if entry["socialProfileType"] != "PINTEREST" {
		t.Errorf("socialProfileType = %v", entry["socialProfileType"])
	}
	data := entry["data"].(map[string]any)
	if data["boardId"] != "12345678909876" || data["destinationUrl"] != "https://example.com" {
		t.Errorf("pinterest data = %v", data)
	}
}

func TestMessageSchedulePinterestRequiresSingleProfile(t *testing.T) {
	exit, _, _ := run(t, nil, "message", "schedule",
		"--text", "x", "--profile", "1", "--profile", "2",
		"--board-id", "b", "--destination-url", "https://e.com")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (Pinterest cannot be bundled)", exit)
	}
}

func TestMessageList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "message", "list",
		"--state", "SCHEDULED",
		"--start", "2029-01-01T00:00:00Z",
		"--end", "2029-02-01T00:00:00Z",
		"--profile", "1", "--profile", "2")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/messages" {
		t.Errorf("path = %s", got.Path)
	}
	q := got.Query
	for _, want := range []string{"state=SCHEDULED", "startTime=2029-01-01T00", "endTime=2029-02-01T00"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q missing %q", q, want)
		}
	}
	if strings.Count(q, "socialProfileIds=") != 2 {
		t.Errorf("query %q should carry two socialProfileIds", q)
	}
}

func TestMessageListRejectsNonZStart(t *testing.T) {
	exit, _, _ := run(t, nil, "message", "list", "--start", "2029-01-01T00:00:00+02:00")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", exit)
	}
}

func TestMessageGetDeleteApproveReject(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"m1"}}`, &got)
	defer srv.Close()

	if exit, _, _ := run(t, srv, "message", "get", "m1"); exit != 0 {
		t.Fatalf("get exit")
	}
	if got.Method != http.MethodGet || got.Path != "/messages/m1" {
		t.Errorf("get = %s %s", got.Method, got.Path)
	}

	if exit, _, _ := run(t, srv, "message", "delete", "m1"); exit != 0 {
		t.Fatalf("delete exit")
	}
	if got.Method != http.MethodDelete || got.Path != "/messages/m1" {
		t.Errorf("delete = %s %s", got.Method, got.Path)
	}

	if exit, _, _ := run(t, srv, "message", "approve", "m1"); exit != 0 {
		t.Fatalf("approve exit")
	}
	if got.Method != http.MethodPost || got.Path != "/messages/m1/approve" {
		t.Errorf("approve = %s %s", got.Method, got.Path)
	}

	if exit, _, _ := run(t, srv, "message", "reject", "m1", "--reason", "off-brand"); exit != 0 {
		t.Fatalf("reject exit")
	}
	if got.Method != http.MethodPost || got.Path != "/messages/m1/reject" {
		t.Errorf("reject = %s %s", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["reason"] != "off-brand" {
		t.Errorf("reject reason = %v", body["reason"])
	}
}

func TestMediaCreateAndGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"id":"md1","uploadUrl":"https://u"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "media", "create", "--size-bytes", "10240", "--mime-type", "image/png")
	if exit != 0 {
		t.Fatalf("create exit = %d, stderr = %s", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/media" {
		t.Errorf("create = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["sizeBytes"].(float64) != 10240 {
		t.Errorf("sizeBytes = %v", body["sizeBytes"])
	}
	if body["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v", body["mimeType"])
	}

	if exit, _, _ := run(t, srv, "media", "get", "md1"); exit != 0 {
		t.Fatalf("get exit")
	}
	if got.Path != "/media/md1" {
		t.Errorf("get path = %s", got.Path)
	}
}

func TestAPIErrorNon401ExitsOneWithJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest,
		`{"errors":[{"code":1005,"message":"token could not be retrieved"}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "me", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("400 should not reject the credential")
	}
	env := decodeJSON(t, stderr).(map[string]any)
	errObj := env["error"].(map[string]any)
	if errObj["status"].(float64) != 400 {
		t.Errorf("status = %v, want 400", errObj["status"])
	}
	if errObj["code"] != "1005" {
		t.Errorf("code = %v, want 1005", errObj["code"])
	}
	if !strings.Contains(errObj["message"].(string), "token could not be retrieved") {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestAPIError401RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"errors":[{"code":1004,"message":"unauthorized"}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "me")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("401 must reject the credential; stderr = %s", stderr)
	}
}

func TestPlainTextErrorRendering(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"errors":[{"code":1006,"message":"nope"}]}`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "me")
	if strings.HasPrefix(strings.TrimSpace(stderr), "{") {
		t.Errorf("plain-text mode should not emit JSON: %s", stderr)
	}
	if !strings.Contains(stderr, "nope") {
		t.Errorf("stderr missing provider message: %s", stderr)
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A missing injected credential is a runtime/environment precondition
	// failure, not a usage error → exit 1 (never the usage-error exit 2).
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (runtime failure)", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("a never-injected credential is not a provider rejection")
	}
}

// TestMissingTokenRuntimeEnvelope pins the JSON error envelope for a missing
// credential to the runtime (not usage) convention: code must not be the
// usage-error "invalid_request", and no HTTP status is attached.
func TestMissingTokenRuntimeEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (runtime failure)", result.ExitCode)
	}
	env := decodeJSON(t, errBuf.String()).(map[string]any)
	errObj := env["error"].(map[string]any)
	if errObj["code"] == "invalid_request" {
		t.Errorf("missing-credential code must not use the usage-error convention: %v", errObj["code"])
	}
	if errObj["code"] != "unauthenticated" {
		t.Errorf("code = %v, want unauthenticated", errObj["code"])
	}
	if _, hasStatus := errObj["status"]; hasStatus {
		t.Errorf("client-side precondition error must not carry an HTTP status: %v", errObj["status"])
	}
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "HOOTSUITE_ACCESS_TOKEN is not set") {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestUnknownSubcommandExitsTwo(t *testing.T) {
	exit, _, _ := run(t, nil, "bogus")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", exit)
	}
}
