package phantombuster

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records one request the fake server received.
type capturedRequest struct {
	Method string
	Path   string
	Key    string
	Query  map[string][]string
	Body   []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newServer is a multi-route fake PhantomBuster API: it answers each request
// from routes keyed by "METHOD /path" and records every request. An unmatched
// route returns 404 so tests notice a wrong path.
func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Key:    r.Header.Get(authHeader),
			Query:  r.URL.Query(),
			Body:   body,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"unmatched route"}`))
	}))
}

// run executes one command against the fake server and returns stdout, stderr,
// and the service result.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, int, bool) {
	t.Helper()
	var out, errBuf bytes.Buffer
	// Mirror the production base path (.../api/v2) so recorded paths match routes.
	svc := &Service{BaseURL: srv.URL + "/api/v2", HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: "test-key"})
	if err != nil {
		t.Fatalf("Execute returned a Go error (should never happen): %v", err)
	}
	return out.String(), errBuf.String(), res.ExitCode, res.CredentialRejected
}

// decodeEnvelope decodes stdout into the success envelope and fails if ok!=true.
func decodeEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v (%q)", err, stdout)
	}
	if !env.OK {
		t.Fatalf("envelope ok=false, want true: %q", stdout)
	}
	return env.Data
}

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

func TestAgentListInjectsKeyAndFilters(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/agents/fetch-all": {200, `[{"id":"111","name":"Scraper","s3Folder":"abc"},{"id":"222","name":"Enricher"}]`},
	})
	defer srv.Close()

	stdout, stderr, code, _ := run(t, srv, "agent", "list", "--input-types", "linkedin", "--ids", "111,222", "--with-argument")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, stderr)
	}
	req := findReq(reqs, "GET", "/api/v2/agents/fetch-all")
	if req == nil {
		t.Fatal("no fetch-all request")
	}
	if req.Key != "test-key" {
		t.Errorf("X-Phantombuster-Key = %q, want test-key", req.Key)
	}
	if got := req.Query["inputTypes"]; len(got) != 1 || got[0] != "linkedin" {
		t.Errorf("inputTypes = %v", got)
	}
	if got := req.Query["agentIds"]; len(got) != 1 || got[0] != "111,222" {
		t.Errorf("agentIds = %v", got)
	}
	if got := req.Query["withArgument"]; len(got) != 1 || got[0] != "true" {
		t.Errorf("withArgument = %v", got)
	}
	data := decodeEnvelope(t, stdout)
	items, ok := data["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("data.items = %v", data["items"])
	}
}

func TestAgentGetRequiresID(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, stderr, code, _ := run(t, srv, "agent", "get")
	if code != 2 {
		t.Fatalf("missing --id exit = %d, want 2 (usage)", code)
	}
	// Error must be the JSON envelope, not a bare cobra message.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%q)", err, stderr)
	}
	if env["ok"] != false {
		t.Errorf("error envelope ok = %v, want false", env["ok"])
	}
}

func TestAgentLaunchSendsArgumentAndNormalizes(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/v2/agents/launch": {200, `{"containerId":"9988776655"}`},
	})
	defer srv.Close()

	stdout, stderr, code, _ := run(t, srv, "agent", "launch", "--id", "111",
		"--argument", `{"urls":["https://x.com/a"]}`, "--save-argument")
	if code != 0 {
		t.Fatalf("exit = %d stderr=%q", code, stderr)
	}
	req := findReq(reqs, "POST", "/api/v2/agents/launch")
	if req == nil {
		t.Fatal("no launch request")
	}
	var sent map[string]any
	if err := json.Unmarshal(req.Body, &sent); err != nil {
		t.Fatalf("launch body not JSON: %v", err)
	}
	if sent["id"] != "111" {
		t.Errorf("body.id = %v, want 111", sent["id"])
	}
	arg, ok := sent["argument"].(map[string]any)
	if !ok {
		t.Fatalf("body.argument not an object: %v", sent["argument"])
	}
	if _, ok := arg["urls"].([]any); !ok {
		t.Errorf("body.argument.urls missing: %v", arg)
	}
	if sent["saveArgument"] != true {
		t.Errorf("body.saveArgument = %v, want true", sent["saveArgument"])
	}
	data := decodeEnvelope(t, stdout)
	if data["agent_id"] != "111" {
		t.Errorf("data.agent_id = %v, want 111", data["agent_id"])
	}
	if data["containerId"] != "9988776655" {
		t.Errorf("data.containerId = %v (raw field should pass through)", data["containerId"])
	}
}

func TestAgentLaunchInvalidArgumentIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, code, _ := run(t, srv, "agent", "launch", "--id", "111", "--argument", "{not json")
	if code != 2 {
		t.Fatalf("bad --argument exit = %d, want 2 (usage)", code)
	}
	if findReq(reqs, "POST", "/api/v2/agents/launch") != nil {
		t.Error("launch should not be called when --argument is invalid JSON")
	}
}

func TestAgentOutputCursorRoundTrip(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/agents/fetch-output": {200, `{"containerId":"55","status":"running","output":"line","outputPos":4096,"isAgentRunning":true}`},
	})
	defer srv.Close()

	stdout, stderr, code, _ := run(t, srv, "agent", "output", "--id", "111", "--from-pos", "2048")
	if code != 0 {
		t.Fatalf("exit = %d stderr=%q", code, stderr)
	}
	req := findReq(reqs, "GET", "/api/v2/agents/fetch-output")
	if got := req.Query["fromOutputPos"]; len(got) != 1 || got[0] != "2048" {
		t.Errorf("fromOutputPos = %v, want [2048]", got)
	}
	data := decodeEnvelope(t, stdout)
	if data["is_running"] != true {
		t.Errorf("data.is_running = %v, want true", data["is_running"])
	}
	if op, ok := data["output_pos"].(float64); !ok || int(op) != 4096 {
		t.Errorf("data.output_pos = %v, want 4096", data["output_pos"])
	}
}

func TestAgentOutputOmitsCursorWhenUnset(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/agents/fetch-output": {200, `{"outputPos":0,"isAgentRunning":false}`},
	})
	defer srv.Close()

	_, _, code, _ := run(t, srv, "agent", "output", "--id", "111")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	req := findReq(reqs, "GET", "/api/v2/agents/fetch-output")
	if _, present := req.Query["fromOutputPos"]; present {
		t.Error("fromOutputPos should be omitted when --from-pos is unset")
	}
}

func TestContainerGetDerivesIsRunningAndISO(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/containers/fetch": {200, `{"id":"55","status":"finished","endType":"finished","exitCode":0,"createdAt":1700000000000}`},
	})
	defer srv.Close()

	stdout, _, code, _ := run(t, srv, "container", "get", "--id", "55")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	data := decodeEnvelope(t, stdout)
	if data["is_running"] != false {
		t.Errorf("data.is_running = %v, want false for finished", data["is_running"])
	}
	iso, ok := data["createdAt_iso"].(string)
	if !ok || !strings.HasPrefix(iso, "2023-11-14T") {
		t.Errorf("data.createdAt_iso = %v, want an RFC3339 mirror", data["createdAt_iso"])
	}
}

func TestContainerListPassesAgentIDAndPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/containers/fetch-all": {200, `{"maxLimitReached":false,"containers":[{"id":"55","status":"finished"}]}`},
	})
	defer srv.Close()

	stdout, _, code, _ := run(t, srv, "container", "list", "--agent-id", "111")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	req := findReq(reqs, "GET", "/api/v2/containers/fetch-all")
	if got := req.Query["agentId"]; len(got) != 1 || got[0] != "111" {
		t.Errorf("agentId = %v", got)
	}
	data := decodeEnvelope(t, stdout)
	if _, ok := data["containers"].([]any); !ok {
		t.Errorf("data.containers passthrough missing: %v", data)
	}
	if data["maxLimitReached"] != false {
		t.Errorf("data.maxLimitReached = %v", data["maxLimitReached"])
	}
}

func TestContainerResultPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/containers/fetch-result-object": {200, `{"resultObject":"[{\"name\":\"row\"}]"}`},
	})
	defer srv.Close()

	stdout, _, code, _ := run(t, srv, "container", "result", "--id", "55")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	data := decodeEnvelope(t, stdout)
	if _, ok := data["resultObject"].(string); !ok {
		t.Errorf("data.resultObject should be a passthrough string: %v", data["resultObject"])
	}
}

func TestOrgResourcesAndMe(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/orgs/fetch-resources": {200, `{"dailyExecutionTime":3600,"executionTimeUsed":120}`},
		"GET /api/v2/users/fetch-me":       {200, `{"sessionId":"s1","user":{"id":"u1","email":"a@b.co"}}`},
	})
	defer srv.Close()

	stdout, _, code, _ := run(t, srv, "org", "resources")
	if code != 0 {
		t.Fatalf("org resources exit = %d", code)
	}
	if d := decodeEnvelope(t, stdout); d["dailyExecutionTime"] == nil {
		t.Errorf("org resources passthrough missing: %v", d)
	}

	stdout, _, code, _ = run(t, srv, "me")
	if code != 0 {
		t.Fatalf("me exit = %d", code)
	}
	if d := decodeEnvelope(t, stdout); d["user"] == nil {
		t.Errorf("me passthrough missing user: %v", d)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /api/v2/orgs/fetch": {401, `{"error":"invalid api key"}`},
	})
	defer srv.Close()

	_, stderr, code, rejected := run(t, srv, "org", "get")
	if code != 1 {
		t.Fatalf("401 exit = %d, want 1", code)
	}
	if !rejected {
		t.Error("401 should mark the credential rejected")
	}
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%q)", err, stderr)
	}
	if env.OK || env.Error.Code != "api" || env.Error.Status != 401 {
		t.Errorf("error envelope = %+v, want api/401", env.Error)
	}
	if !strings.Contains(env.Error.Message, "invalid api key") {
		t.Errorf("message = %q, want provider text surfaced", env.Error.Message)
	}
}

func TestQuota429IsApiErrorNotRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /api/v2/agents/launch": {429, `{"error":"org execution-time quota exceeded"}`},
	})
	defer srv.Close()

	_, stderr, code, rejected := run(t, srv, "agent", "launch", "--id", "111")
	if code != 1 {
		t.Fatalf("429 exit = %d, want 1", code)
	}
	if rejected {
		t.Error("429 quota must NOT mark the credential rejected")
	}
	if !strings.Contains(stderr, "429") || !strings.Contains(stderr, "quota") {
		t.Errorf("stderr = %q, want 429 + quota surfaced", stderr)
	}
}

func TestMissingKeyExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"org", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("missing key exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAPIKey) {
		t.Errorf("stderr = %q, want mention of %s", errBuf.String(), EnvAPIKey)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, code, _ := run(t, srv, "agent", "frobnicate")
	if code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
	if len(reqs) != 0 {
		t.Errorf("unknown subcommand should make no API call, got %d", len(reqs))
	}
}
