package facebookpages

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const (
	userToken = "USER-LONG-LIVED-TOKEN"
	pageToken = "PAGE-TOKEN-SECRET"
	pageID    = "1122334455"
)

type capturedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Auth     string
	Body     string
}

// twoHopServer routes the Page-token resolution (GET /{pageID}?fields=access_token
// with the user token) to a Page token, and records every request so tests can
// assert on the header swap. The op handler is invoked for the actual call.
func twoHopServer(t *testing.T, op http.HandlerFunc) (*httptest.Server, *[]capturedRequest) {
	t.Helper()
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqs = append(reqs, capturedRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
			Auth:     r.Header.Get("Authorization"),
			Body:     string(body),
		})
		// First leg: Page-token resolution.
		if r.Method == http.MethodGet && r.URL.Path == "/"+pageID && r.URL.Query().Get("fields") == "access_token" {
			writeJSON(w, http.StatusOK, `{"access_token":"`+pageToken+`","id":"`+pageID+`"}`)
			return
		}
		op(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func run(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &stdout, Err: &stderr}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return res, stdout.String(), stderr.String()
}

func fullEnv() map[string]string { return map[string]string{EnvAccessToken: userToken} }

func TestPagesListUsesUserTokenAndOmitsAccessToken(t *testing.T) {
	srv, reqs := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/accounts" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{"data":[{"id":"`+pageID+`","name":"Acme","category":"Business","tasks":["CREATE_CONTENT"]}]}`)
	})
	res, stdout, stderr := run(t, srv, fullEnv(), "pages", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", res.ExitCode, stderr)
	}
	got := (*reqs)[0]
	if got.Auth != "Bearer "+userToken {
		t.Errorf("auth = %q, want user token bearer", got.Auth)
	}
	// The discovery call must NOT request the per-Page access_token field.
	if strings.Contains(got.RawQuery, "access_token") {
		t.Errorf("pages list query %q must not request access_token", got.RawQuery)
	}
	if fields := decodeQuery(t, got.RawQuery).Get("fields"); fields != "id,name,category,tasks" {
		t.Errorf("fields = %q, want id,name,category,tasks", fields)
	}
	if !strings.Contains(stdout, `"Acme"`) {
		t.Errorf("stdout missing page data: %q", stdout)
	}
}

func TestPostCreatePerformsTwoHopAndSwapsToken(t *testing.T) {
	srv, reqs := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/"+pageID+"/feed" {
			t.Fatalf("unexpected op %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{"id":"`+pageID+`_9988"}`)
	})
	res, stdout, stderr := run(t, srv, fullEnv(),
		"post", "create", "--page", pageID, "--message", "hello world", "--link", "https://example.com")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", res.ExitCode, stderr)
	}
	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests (resolve + publish), got %d", len(*reqs))
	}
	resolve, publish := (*reqs)[0], (*reqs)[1]
	// First leg uses the USER token.
	if resolve.Auth != "Bearer "+userToken {
		t.Errorf("resolve auth = %q, want user token", resolve.Auth)
	}
	// Second leg (the actual publish) must carry the derived PAGE token — the swap.
	if publish.Auth != "Bearer "+pageToken {
		t.Errorf("publish auth = %q, want swapped page token", publish.Auth)
	}
	form := decodeQuery(t, publish.Body)
	if form.Get("message") != "hello world" || form.Get("link") != "https://example.com" {
		t.Errorf("publish body = %q, want message+link", publish.Body)
	}
	// Response is projected to a compact {"id":...}; the Page token must never leak.
	if strings.Contains(stdout, pageToken) {
		t.Fatalf("page token leaked into stdout: %q", stdout)
	}
	var out struct{ ID string }
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &out); err != nil || out.ID != pageID+"_9988" {
		t.Errorf("stdout = %q, want {id:...}", stdout)
	}
}

func TestPageScopedCommandRequiresPageFlag(t *testing.T) {
	srv, _ := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected when --page is missing")
	})
	res, _, stderr := run(t, srv, fullEnv(), "post", "list")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage) stderr=%q", res.ExitCode, stderr)
	}
	if !strings.Contains(stderr, "page") {
		t.Errorf("stderr should mention the required page flag: %q", stderr)
	}
}

func TestPostCreateRequiresMessageOrLink(t *testing.T) {
	srv, _ := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected for an empty create")
	})
	res, _, stderr := run(t, srv, fullEnv(), "post", "create", "--page", pageID)
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage) stderr=%q", res.ExitCode, stderr)
	}
	if !strings.Contains(stderr, "message") {
		t.Errorf("stderr should explain the message/link requirement: %q", stderr)
	}
}

func TestCommentHideSendsIsHidden(t *testing.T) {
	srv, reqs := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/comment_42" {
			t.Fatalf("unexpected op %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{"success":true}`)
	})
	res, _, stderr := run(t, srv, fullEnv(), "comment", "hide", "comment_42", "--page", pageID)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", res.ExitCode, stderr)
	}
	publish := (*reqs)[1]
	if publish.Auth != "Bearer "+pageToken {
		t.Errorf("hide auth = %q, want page token", publish.Auth)
	}
	if decodeQuery(t, publish.Body).Get("is_hidden") != "true" {
		t.Errorf("hide body = %q, want is_hidden=true", publish.Body)
	}
}

func TestCommentUnhideSendsIsHiddenFalse(t *testing.T) {
	srv, reqs := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, `{"success":true}`)
	})
	run(t, srv, fullEnv(), "comment", "hide", "comment_42", "--page", pageID, "--hidden=false")
	if decodeQuery(t, (*reqs)[1].Body).Get("is_hidden") != "false" {
		t.Errorf("unhide body = %q, want is_hidden=false", (*reqs)[1].Body)
	}
}

func TestInsightsQueryParams(t *testing.T) {
	srv, reqs := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+pageID+"/insights" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{"data":[]}`)
	})
	run(t, srv, fullEnv(), "insights", "--page", pageID,
		"--metrics", "page_impressions", "--period", "week", "--since", "100", "--until", "200")
	q := decodeQuery(t, (*reqs)[1].RawQuery)
	if q.Get("metric") != "page_impressions" || q.Get("period") != "week" || q.Get("since") != "100" || q.Get("until") != "200" {
		t.Errorf("insights query = %q, want metric/period/since/until", (*reqs)[1].RawQuery)
	}
}

func TestExpiredTokenIsCredentialRejection(t *testing.T) {
	// code 190 on the first (resolve) leg — the connection is dead.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest,
			`{"error":{"message":"Error validating access token","type":"OAuthException","code":190,"fbtrace_id":"AbC"}}`)
	}))
	t.Cleanup(srv.Close)
	res, _, stderr := run(t, srv, fullEnv(), "page", "get", "--page", pageID)
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 + credential rejection", res)
	}
	if !strings.Contains(stderr, "reconnect") {
		t.Errorf("stderr should prompt reconnect: %q", stderr)
	}
	// The failing leg is the token resolution — reported distinctly.
	if !strings.Contains(stderr, "resolve Page access token") {
		t.Errorf("stderr should attribute the failure to the resolve leg: %q", stderr)
	}
}

func TestPermissionErrorIsNotCredentialRejection(t *testing.T) {
	// Resolve succeeds; the publish fails with code 200 (insufficient permission).
	srv, _ := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden,
			`{"error":{"message":"(#200) Insufficient permission","type":"OAuthException","code":200}}`)
	})
	res, _, stderr := run(t, srv, fullEnv(), "post", "create", "--page", pageID, "--message", "x")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 WITHOUT credential rejection", res)
	}
	if !strings.Contains(stderr, "permission") {
		t.Errorf("stderr should mention insufficient permission: %q", stderr)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	srv, _ := twoHopServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, `{"error":{"message":"Unknown post","type":"GraphMethodException","code":803}}`)
	})
	res, _, stderr := run(t, srv, fullEnv(), "post", "get", "post_1", "--page", pageID, "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var envelope struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %q (%v)", stderr, err)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != http.StatusNotFound {
		t.Errorf("envelope = %+v, want kind=api status=404", envelope.Error)
	}
}

func TestMissingTokenFailsFast(t *testing.T) {
	var stdout, stderr bytes.Buffer
	svc := &Service{Out: &stdout, Err: &stderr}
	res, err := svc.Execute(context.Background(), []string{"pages", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(stderr.String(), EnvAccessToken) {
		t.Errorf("stderr should name the missing env var: %q", stderr.String())
	}
}

func TestResolveFailureReportedDistinctlyFromOp(t *testing.T) {
	// The resolve leg fails with a non-190 error (e.g. unknown page id) — the op
	// must never run, and the message must attribute it to the resolve leg.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest,
			`{"error":{"message":"Unsupported get request; object does not exist","type":"GraphMethodException","code":100}}`)
	}))
	t.Cleanup(srv.Close)
	res, _, stderr := run(t, srv, fullEnv(), "post", "list", "--page", "does_not_exist")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(stderr, "resolve Page access token for does_not_exist") {
		t.Errorf("stderr should attribute failure to the resolve leg: %q", stderr)
	}
}

// decodeQuery parses a raw query or form-encoded body into url.Values.
func decodeQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return v
}
