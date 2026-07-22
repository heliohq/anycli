package instagram

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

// capturedRequest records one request the fake Graph server received.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Query  url.Values
	Form   url.Values
	Body   []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

func newMux(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = r.ParseForm()
		form := url.Values{}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			form, _ = url.ParseQuery(string(raw))
		}
		*reqs = append(*reqs, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Query:  r.URL.Query(),
			Form:   form,
			Body:   raw,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = io.WriteString(w, s.body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"Unknown path","type":"GraphMethodException","code":100}}`)
	}))
}

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

// run executes one instagram invocation against srv, returning stdout, exit
// code, and credential-rejection flag.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "IGQVJ-token"})
	if err != nil {
		t.Fatalf("Execute returned a transport error (should be nil): %v", err)
	}
	return out.String(), errBuf.String(), res
}

func TestAccountGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {http.StatusOK, `{"user_id":"178414","username":"acme"}`},
	})
	defer srv.Close()

	out, _, res := run(t, srv, "account", "get")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "GET", "/me")
	if req == nil {
		t.Fatalf("no GET /me request; got %+v", reqs)
	}
	if req.Auth != "Bearer IGQVJ-token" {
		t.Errorf("auth = %q, want Bearer IGQVJ-token", req.Auth)
	}
	if got := req.Query.Get("fields"); got != accountFields {
		t.Errorf("fields = %q, want %q", got, accountFields)
	}
	if !strings.Contains(out, "acme") {
		t.Errorf("stdout missing body: %q", out)
	}
}

func TestMediaListParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me/media": {http.StatusOK, `{"data":[]}`},
	})
	defer srv.Close()

	_, _, res := run(t, srv, "media", "list", "--limit", "5", "--after", "CUR", "--fields", "id,caption")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "GET", "/me/media")
	if req == nil {
		t.Fatalf("no media list request; got %+v", reqs)
	}
	if req.Query.Get("limit") != "5" || req.Query.Get("after") != "CUR" {
		t.Errorf("pagination params wrong: %v", req.Query)
	}
	if req.Query.Get("fields") != "id,caption" {
		t.Errorf("fields override lost: %q", req.Query.Get("fields"))
	}
}

func TestMediaListDefaultFields(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /me/media": {http.StatusOK, `{"data":[]}`}})
	defer srv.Close()
	run(t, srv, "media", "list")
	req := findReq(reqs, "GET", "/me/media")
	if req == nil || req.Query.Get("fields") != mediaFields {
		t.Fatalf("default media fields not applied: %+v", req)
	}
}

func TestMediaInsightsMetrics(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /17895/insights": {http.StatusOK, `{"data":[]}`},
	})
	defer srv.Close()
	run(t, srv, "media", "insights", "17895")
	req := findReq(reqs, "GET", "/17895/insights")
	if req == nil || req.Query.Get("metric") != mediaInsightMetrics {
		t.Fatalf("media insights metric wrong: %+v", req)
	}
}

func TestPublishCreateContainer(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /me/media": {http.StatusOK, `{"id":"17999"}`},
	})
	defer srv.Close()

	out, _, res := run(t, srv, "publish", "create", "--image-url", "https://x/y.jpg", "--caption", "hi", "--media-type", "IMAGE")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "POST", "/me/media")
	if req == nil {
		t.Fatalf("no POST /me/media; got %+v", reqs)
	}
	if req.Form.Get("image_url") != "https://x/y.jpg" || req.Form.Get("caption") != "hi" || req.Form.Get("media_type") != "IMAGE" {
		t.Errorf("form body wrong: %v", req.Form)
	}
	if !strings.Contains(out, "17999") {
		t.Errorf("container id not surfaced: %q", out)
	}
}

func TestPublishCreateRequiresExactlyOneURL(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	// Neither URL.
	if _, _, res := run(t, srv, "publish", "create"); res.ExitCode != 2 {
		t.Errorf("no URL: exit = %d, want 2", res.ExitCode)
	}
	// Both URLs.
	if _, _, res := run(t, srv, "publish", "create", "--image-url", "a", "--video-url", "b"); res.ExitCode != 2 {
		t.Errorf("both URLs: exit = %d, want 2", res.ExitCode)
	}
	if len(reqs) != 0 {
		t.Errorf("usage errors must not hit the API; got %d requests", len(reqs))
	}
}

func TestPublishCreateBadMediaType(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	if _, _, res := run(t, srv, "publish", "create", "--image-url", "a", "--media-type", "CAROUSEL"); res.ExitCode != 2 {
		t.Errorf("bad media-type: exit = %d, want 2", res.ExitCode)
	}
}

func TestPublishStatusFinished(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /17999": {http.StatusOK, `{"status_code":"FINISHED","id":"17999"}`},
	})
	defer srv.Close()
	out, _, res := run(t, srv, "publish", "status", "17999")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "GET", "/17999")
	if req == nil || req.Query.Get("fields") != "status_code" {
		t.Fatalf("status request wrong: %+v", req)
	}
	if !strings.Contains(out, "FINISHED") {
		t.Errorf("status not surfaced: %q", out)
	}
}

func TestPublishStatusErrorExit1(t *testing.T) {
	for _, code := range []string{"ERROR", "EXPIRED"} {
		var reqs []capturedRequest
		srv := newMux(t, &reqs, map[string]stub{
			"GET /17999": {http.StatusOK, `{"status_code":"` + code + `","id":"17999"}`},
		})
		out, errOut, res := run(t, srv, "publish", "status", "17999")
		srv.Close()
		if res.ExitCode != 1 {
			t.Errorf("%s: exit = %d, want 1", code, res.ExitCode)
		}
		if out != "" {
			t.Errorf("%s: terminal container must not emit success body: %q", code, out)
		}
		if !strings.Contains(errOut, code) {
			t.Errorf("%s: error missing status: %q", code, errOut)
		}
	}
}

func TestPublishFinish(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /me/media_publish": {http.StatusOK, `{"id":"18100"}`},
	})
	defer srv.Close()
	out, _, res := run(t, srv, "publish", "finish", "17999")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "POST", "/me/media_publish")
	if req == nil || req.Form.Get("creation_id") != "17999" {
		t.Fatalf("creation_id body wrong: %+v", req)
	}
	if !strings.Contains(out, "18100") {
		t.Errorf("published id not surfaced: %q", out)
	}
}

func TestCommentReply(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /555/replies": {http.StatusOK, `{"id":"666"}`},
	})
	defer srv.Close()
	_, _, res := run(t, srv, "comment", "reply", "555", "--message", "thanks!")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	req := findReq(reqs, "POST", "/555/replies")
	if req == nil || req.Form.Get("message") != "thanks!" {
		t.Fatalf("reply body wrong: %+v", req)
	}
}

func TestCommentReplyRequiresMessage(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	if _, _, res := run(t, srv, "comment", "reply", "555"); res.ExitCode != 2 {
		t.Errorf("exit = %d, want 2", res.ExitCode)
	}
}

func TestCommentHide(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"POST /555": {http.StatusOK, `{"success":true}`}})
	defer srv.Close()
	run(t, srv, "comment", "hide", "555", "--hidden", "false")
	req := findReq(reqs, "POST", "/555")
	if req == nil || req.Form.Get("hide") != "false" {
		t.Fatalf("hide body wrong: %+v", req)
	}
}

func TestCommentDelete(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"DELETE /555": {http.StatusOK, `{"success":true}`}})
	defer srv.Close()
	_, _, res := run(t, srv, "comment", "delete", "555")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if findReq(reqs, "DELETE", "/555") == nil {
		t.Fatalf("no DELETE /555; got %+v", reqs)
	}
}

func TestAccountInsightsParams(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /me/insights": {http.StatusOK, `{"data":[]}`}})
	defer srv.Close()
	run(t, srv, "insights", "--since", "100", "--until", "200")
	req := findReq(reqs, "GET", "/me/insights")
	if req == nil {
		t.Fatalf("no insights request; got %+v", reqs)
	}
	if req.Query.Get("metric") != accountInsightMetrics || req.Query.Get("period") != "day" {
		t.Errorf("insights defaults wrong: %v", req.Query)
	}
	// profile_views was deprecated in Graph v22.0 (the service pins v23.0), so
	// it must not appear in the built-in default metric set.
	if strings.Contains(accountInsightMetrics, "profile_views") {
		t.Errorf("default account metrics still include deprecated profile_views: %q", accountInsightMetrics)
	}
	// metric_type is a passthrough: absent unless the caller supplies it, so a
	// time_series metric (e.g. follower_count) is not forced onto total_value.
	if req.Query.Has("metric_type") {
		t.Errorf("metric_type must not default: %v", req.Query)
	}
	if req.Query.Get("since") != "100" || req.Query.Get("until") != "200" {
		t.Errorf("since/until lost: %v", req.Query)
	}
}

// TestAccountInsightsMetricType pins the --metric-type passthrough. Current
// Graph versions (v22+/v23) require metric_type=total_value for several
// account-level metrics; the flag lets the assistant supply it without falling
// back to the raw API.
func TestAccountInsightsMetricType(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{"GET /me/insights": {http.StatusOK, `{"data":[]}`}})
	defer srv.Close()
	run(t, srv, "insights", "--metrics", "reach", "--metric-type", "total_value")
	req := findReq(reqs, "GET", "/me/insights")
	if req == nil {
		t.Fatalf("no insights request; got %+v", reqs)
	}
	if req.Query.Get("metric") != "reach" || req.Query.Get("metric_type") != "total_value" {
		t.Errorf("metric_type not passed through: %v", req.Query)
	}
}

func TestGraphErrorExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {http.StatusBadRequest, `{"error":{"message":"Invalid parameter","type":"OAuthException","code":100,"error_subcode":33}}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, "account", "get")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Errorf("code 100 must not be a credential rejection")
	}
	if !strings.Contains(errOut, "Invalid parameter") {
		t.Errorf("error text missing: %q", errOut)
	}
}

func TestGraphErrorJSONEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {http.StatusBadRequest, `{"error":{"message":"Boom","type":"GraphMethodException","code":100}}`},
	})
	defer srv.Close()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, _ := svc.Execute(context.Background(), []string{"account", "get", "--json"}, map[string]string{EnvToken: "t"})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, errBuf.String())
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("envelope kind/status wrong: %+v", env.Error)
	}
}

func TestOAuthException190Reconnect(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {http.StatusBadRequest, `{"error":{"message":"Error validating access token","type":"OAuthException","code":190}}`},
	})
	defer srv.Close()
	_, errOut, res := run(t, srv, "account", "get")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Errorf("code 190 must set CredentialRejected")
	}
	if !strings.Contains(errOut, "reconnect") {
		t.Errorf("reconnect guidance missing: %q", errOut)
	}
}

func Test401Reconnect(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /me": {http.StatusUnauthorized, `{"error":{"message":"Unauthorized","code":10}}`},
	})
	defer srv.Close()
	_, _, res := run(t, srv, "account", "get")
	if !res.CredentialRejected {
		t.Errorf("HTTP 401 must set CredentialRejected")
	}
}

func TestMissingTokenExit1(t *testing.T) {
	svc := &Service{Out: io.Discard, Err: io.Discard}
	res, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("missing token: exit = %d, want 1", res.ExitCode)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	if _, _, res := run(t, srv, "media", "bogus"); res.ExitCode != 2 {
		t.Errorf("unknown subcommand: exit = %d, want 2", res.ExitCode)
	}
}
