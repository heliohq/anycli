package moz

import (
	"encoding/json"
	"testing"
)

// okResult is a minimal valid JSON-RPC success body for shape-only assertions.
const okResult = `{"jsonrpc":"2.0","id":"x","result":{"ok":true}}`

// TestSiteMetricsSingle asserts one --site routes to data.site.metrics.fetch
// with a site_query and passes scope through.
func TestSiteMetricsSingle(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "site", "metrics", "--site", "moz.com", "--scope", "root_domain")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr %q", exit, stderr)
	}
	env := decodeRPC(t, got.Body)
	if env.Method != "data.site.metrics.fetch" {
		t.Fatalf("method = %q, want data.site.metrics.fetch", env.Method)
	}
	data := decodeData(t, env)
	sq, ok := data["site_query"].(map[string]any)
	if !ok {
		t.Fatalf("params.data.site_query missing: %v", data)
	}
	if sq["query"] != "moz.com" || sq["scope"] != "root_domain" {
		t.Errorf("site_query = %v, want query=moz.com scope=root_domain", sq)
	}
}

// TestSiteMetricsMultiple asserts repeated --site routes to
// data.site.metrics.fetch.multiple with a site_queries array.
func TestSiteMetricsMultiple(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "site", "metrics", "--site", "moz.com", "--site", "ahrefs.com")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	env := decodeRPC(t, got.Body)
	if env.Method != "data.site.metrics.fetch.multiple" {
		t.Fatalf("method = %q, want data.site.metrics.fetch.multiple", env.Method)
	}
	data := decodeData(t, env)
	arr, ok := data["site_queries"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("site_queries = %v, want 2 entries", data["site_queries"])
	}
}

// TestSiteMetricsRequiresSite asserts a missing --site is a usage error.
func TestSiteMetricsRequiresSite(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "site", "metrics")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for missing --site", exit)
	}
}

// TestInvalidScopeExit2 asserts a bad --scope value is rejected locally (exit
// 2). "domain" (not a Moz scope) is the classic mistake — the real values are
// page|subdomain|root_domain.
func TestInvalidScopeExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "site", "metrics", "--site", "moz.com", "--scope", "domain")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (stderr %q)", exit, stderr)
	}
}

// TestBrandAuthority asserts brand-authority routes to the domain-level method
// with a bare site_query (no scope).
func TestBrandAuthority(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "site", "brand-authority", "--site", "moz.com")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	env := decodeRPC(t, got.Body)
	if env.Method != "data.site.metrics.brand.authority.fetch" {
		t.Errorf("method = %q, want brand authority", env.Method)
	}
}

// TestTargetListCommands table-drives the target_query-scoped list commands,
// asserting method routing, the target_query, and the default limit.
func TestTargetListCommands(t *testing.T) {
	cases := []struct {
		args   []string
		method string
	}{
		{[]string{"link", "list", "--site", "moz.com"}, "data.site.link.list"},
		{[]string{"link", "domains", "--site", "moz.com"}, "data.site.linking-domain.list"},
		{[]string{"link", "anchors", "--site", "moz.com"}, "data.site.anchor-text.list"},
		{[]string{"site", "top-pages", "--site", "moz.com"}, "data.site.top-page.list"},
		{[]string{"ranking-keywords", "list", "--site", "moz.com"}, "data.site.ranking-keyword.list"},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, 200, okResult, &got)
			defer srv.Close()

			exit, _, stderr := run(t, srv, tc.args...)
			if exit != 0 {
				t.Fatalf("exit = %d, stderr %q", exit, stderr)
			}
			env := decodeRPC(t, got.Body)
			if env.Method != tc.method {
				t.Fatalf("method = %q, want %q", env.Method, tc.method)
			}
			data := decodeData(t, env)
			if _, ok := data["target_query"].(map[string]any); !ok {
				t.Errorf("params.data.target_query missing: %v", data)
			}
			if data["limit"] != float64(25) {
				t.Errorf("limit = %v, want default 25", data["limit"])
			}
		})
	}
}

// TestRankingKeywordsCount asserts the count command routes correctly with a
// target_query and no limit.
func TestRankingKeywordsCount(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "ranking-keywords", "count", "--site", "moz.com")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	env := decodeRPC(t, got.Body)
	if env.Method != "data.site.ranking-keyword.count" {
		t.Errorf("method = %q, want ranking-keyword.count", env.Method)
	}
}

// TestKeywordCommands asserts the serp_query-scoped keyword commands route and
// carry keyword + locale.
func TestKeywordCommands(t *testing.T) {
	cases := []struct {
		args   []string
		method string
	}{
		{[]string{"keyword", "metrics", "--keyword", "seo", "--locale", "en-US"}, "data.keyword.metrics.fetch"},
		{[]string{"keyword", "suggestions", "--keyword", "seo"}, "data.keyword.suggestions.list"},
		{[]string{"keyword", "intent", "--keyword", "seo"}, "data.keyword.search.intent.fetch"},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, 200, okResult, &got)
			defer srv.Close()

			exit, _, stderr := run(t, srv, tc.args...)
			if exit != 0 {
				t.Fatalf("exit = %d, stderr %q", exit, stderr)
			}
			env := decodeRPC(t, got.Body)
			if env.Method != tc.method {
				t.Fatalf("method = %q, want %q", env.Method, tc.method)
			}
			data := decodeData(t, env)
			sq, ok := data["serp_query"].(map[string]any)
			if !ok || sq["keyword"] != "seo" {
				t.Errorf("serp_query = %v, want keyword=seo", data["serp_query"])
			}
		})
	}
}

// TestKeywordMetricsRequiresKeyword asserts a missing --keyword is a usage
// error.
func TestKeywordMetricsRequiresKeyword(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "keyword", "metrics")
	if exit != 2 {
		t.Errorf("exit = %d, want 2", exit)
	}
}

// TestQuotaDefaultPath asserts quota.lookup sends the default row path.
func TestQuotaDefaultPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "quota")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	env := decodeRPC(t, got.Body)
	if env.Method != "quota.lookup" {
		t.Fatalf("method = %q, want quota.lookup", env.Method)
	}
	data := decodeData(t, env)
	if data["path"] != "api.limits.data.rows" {
		t.Errorf("path = %v, want api.limits.data.rows", data["path"])
	}
}

// TestIndexNoParams asserts metadata.index.fetch routes with an empty data
// object.
func TestIndexNoParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "index")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	env := decodeRPC(t, got.Body)
	if env.Method != "metadata.index.fetch" {
		t.Errorf("method = %q, want metadata.index.fetch", env.Method)
	}
}

// TestCallGeneric asserts the generic call passes an arbitrary method and raw
// --data through the envelope verbatim.
func TestCallGeneric(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "call", "--method", "data.site.link.status.fetch", "--data", `{"target_query":{"query":"x"}}`)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr %q", exit, stderr)
	}
	env := decodeRPC(t, got.Body)
	if env.Method != "data.site.link.status.fetch" {
		t.Fatalf("method = %q, want the passthrough method", env.Method)
	}
	var data map[string]any
	if err := json.Unmarshal(env.Params.Data, &data); err != nil {
		t.Fatalf("data not JSON: %v", err)
	}
	if _, ok := data["target_query"].(map[string]any); !ok {
		t.Errorf("params.data = %v, want the raw --data passthrough", data)
	}
}

// TestCallBadDataExit2 asserts invalid --data JSON is a usage error.
func TestCallBadDataExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "call", "--method", "x", "--data", "{not json")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for bad --data", exit)
	}
}

// TestCallRequiresMethod asserts a missing --method is a usage error.
func TestCallRequiresMethod(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, okResult, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "call")
	if exit != 2 {
		t.Errorf("exit = %d, want 2", exit)
	}
}
