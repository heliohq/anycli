package beehiiv

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicationList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[{"id":"pub_1"}]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "publication", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != "GET" || got.Path != "/publications" {
		t.Errorf("request = %s %s, want GET /publications", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("auth = %q, want Bearer tok-123", got.Auth)
	}
	if !strings.Contains(stdout, `"pub_1"`) {
		t.Errorf("stdout missing provider body: %s", stdout)
	}
}

func TestPublicationGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"pub_abc"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "publication", "get", "pub_abc")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/publications/pub_abc" {
		t.Errorf("path = %s, want /publications/pub_abc", got.Path)
	}
}

func TestPostListWithFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "post", "list",
		"--publication-id", "pub_x",
		"--expand", "stats", "--expand", "free_web_content",
		"--status", "confirmed", "--limit", "25", "--page", "2",
		"--order-by", "publish_date", "--direction", "desc",
		"--audience", "all", "--platform", "email")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/publications/pub_x/posts" {
		t.Errorf("path = %s, want /publications/pub_x/posts", got.Path)
	}
	q := parseQuery(t, got.Query)
	if exp := q["expand[]"]; len(exp) != 2 || exp[0] != "stats" || exp[1] != "free_web_content" {
		t.Errorf("expand[] = %v, want [stats free_web_content]", exp)
	}
	if q.Get("status") != "confirmed" || q.Get("limit") != "25" || q.Get("page") != "2" {
		t.Errorf("query filters wrong: %s", got.Query)
	}
	if q.Get("order_by") != "publish_date" || q.Get("direction") != "desc" {
		t.Errorf("order/direction wrong: %s", got.Query)
	}
	if q.Get("audience") != "all" || q.Get("platform") != "email" {
		t.Errorf("audience/platform wrong: %s", got.Query)
	}
}

func TestPostListOmitsUnsetFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "post", "list", "--publication-id", "pub_x")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Query != "" {
		t.Errorf("query = %q, want empty when no filters set", got.Query)
	}
}

func TestPostGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"post_1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "post", "get", "post_1", "--publication-id", "pub_x", "--expand", "stats")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/publications/pub_x/posts/post_1" {
		t.Errorf("path = %s, want /publications/pub_x/posts/post_1", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("expand[]") != "stats" {
		t.Errorf("expand[] = %q, want stats", q.Get("expand[]"))
	}
}

func TestSubscriptionGetByEmailURLEncodesEmail(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"sub_1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "subscription", "get-by-email", "work+tag@example.com",
		"--publication-id", "pub_x", "--expand", "stats")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	// The decoded path the server saw must carry the raw email.
	if got.Path != "/publications/pub_x/subscriptions/by_email/work+tag@example.com" {
		t.Errorf("decoded path = %s", got.Path)
	}
	// The wire (escaped) path must percent-encode @ and +.
	if !strings.Contains(got.RawPath, "work%2Btag%40example.com") {
		t.Errorf("escaped path = %s, want work%%2Btag%%40example.com", got.RawPath)
	}
	if q := parseQuery(t, got.Query); q.Get("expand[]") != "stats" {
		t.Errorf("expand[] = %q", q.Get("expand[]"))
	}
}

func TestSubscriptionList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "subscription", "list",
		"--publication-id", "pub_x", "--status", "active", "--tier", "premium",
		"--limit", "50", "--email", "a@b.com")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/publications/pub_x/subscriptions" {
		t.Errorf("path = %s", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("status") != "active" || q.Get("tier") != "premium" || q.Get("limit") != "50" || q.Get("email") != "a@b.com" {
		t.Errorf("query = %s", got.Query)
	}
}

func TestSubscriptionCreateBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{"id":"sub_new"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "subscription", "create",
		"--publication-id", "pub_x", "--email", "new@example.com",
		"--data", `{"reactivate_existing":true,"tier":"free"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != "POST" || got.Path != "/publications/pub_x/subscriptions" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "new@example.com" {
		t.Errorf("email = %v, want new@example.com", body["email"])
	}
	if body["reactivate_existing"] != true || body["tier"] != "free" {
		t.Errorf("merged body wrong: %v", body)
	}
}

func TestSubscriptionCreateEmailFlagWinsOverData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"data":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "subscription", "create",
		"--publication-id", "pub_x", "--email", "flag@example.com",
		"--data", `{"email":"data@example.com"}`)
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if body := decodeBody(t, got.Body); body["email"] != "flag@example.com" {
		t.Errorf("email = %v, want flag@example.com (flag wins)", body["email"])
	}
}

func TestSubscriptionCreateInvalidDataIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "subscription", "create",
		"--publication-id", "pub_x", "--email", "x@y.com", "--data", `{not json`)
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage); stderr=%s", exit, stderr)
	}
}

func TestSubscriptionUpdateIsPUT(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"data":{"id":"sub_1"}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "subscription", "update", "sub_1",
		"--publication-id", "pub_x", "--data", `{"tier":"premium"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != "PUT" || got.Path != "/publications/pub_x/subscriptions/sub_1" {
		t.Errorf("request = %s %s, want PUT /publications/pub_x/subscriptions/sub_1", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["tier"] != "premium" {
		t.Errorf("body = %v", body)
	}
}

func TestSubscriptionUpdateRequiresData(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "subscription", "update", "sub_1", "--publication-id", "pub_x")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage: empty body)", exit)
	}
}

func TestResourceListPaths(t *testing.T) {
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"segment", "list", "--publication-id", "pub_x"}, "/publications/pub_x/segments"},
		{[]string{"custom-field", "list", "--publication-id", "pub_x"}, "/publications/pub_x/custom_fields"},
		{[]string{"tier", "list", "--publication-id", "pub_x"}, "/publications/pub_x/tiers"},
		{[]string{"automation", "list", "--publication-id", "pub_x"}, "/publications/pub_x/automations"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, 200, `{"data":[]}`, &got)
			defer srv.Close()
			exit, _, stderr := run(t, srv, tc.args...)
			if exit != 0 {
				t.Fatalf("exit = %d, stderr=%s", exit, stderr)
			}
			if got.Path != tc.path {
				t.Errorf("path = %s, want %s", got.Path, tc.path)
			}
		})
	}
}

func TestPublicationIDPrefixGuard(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "post", "list", "--publication-id", "wrongid")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (usage) for bad publication id; stderr=%s", exit, stderr)
	}
	if got.Method != "" {
		t.Errorf("no request should have been made, saw %s %s", got.Method, got.Path)
	}
}

func TestMissingPublicationIDFlag(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "post", "list")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (missing required flag)", exit)
	}
}

func TestMissingTokenExit1(t *testing.T) {
	result, _, stderr := runNoToken(t, "publication", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "BEEHIIV_API_KEY") {
		t.Errorf("stderr = %q, want mention of BEEHIIV_API_KEY", stderr)
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	result, _, stderr := runNoToken(t, "publication", "list", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestAPIErrorPlaintext(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 404, `{"errors":[{"message":"Publication not found"}]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "publication", "get", "pub_missing")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr, "404") || !strings.Contains(stderr, "Publication not found") {
		t.Errorf("stderr = %q, want HTTP 404 + provider message", stderr)
	}
}

func TestAPIErrorJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 422, `{"errors":[{"message":"email is invalid"}]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "subscription", "create",
		"--publication-id", "pub_x", "--email", "bad", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("envelope = %+v, want kind=api status=422", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"errors":[{"message":"Unauthorized"}]}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "publication", "list")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 + credential rejected", result)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "publication", "bogus")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", exit)
	}
}
