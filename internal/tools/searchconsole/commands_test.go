package searchconsole

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSitesList_BearerAndJSON asserts Bearer injection and that the provider's
// siteEntry array is normalized to {"sites":[...]} under --json.
func TestSitesList_BearerAndJSON(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /webmasters/v3/sites": {200, `{"siteEntry":[{"siteUrl":"https://example.com/","permissionLevel":"siteOwner"},{"siteUrl":"sc-domain:example.com","permissionLevel":"siteFullUser"}]}`},
	})
	stdout := f.runOK(t, "sites", "list", "--json")

	req := f.last(t, "GET", "/webmasters/v3/sites")
	if req.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want Bearer ya29.test-token", req.Auth)
	}
	var got struct {
		Sites []struct {
			SiteURL         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"sites"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if len(got.Sites) != 2 || got.Sites[0].SiteURL != "https://example.com/" || got.Sites[1].PermissionLevel != "siteFullUser" {
		t.Errorf("sites = %+v, want the two normalized rows", got.Sites)
	}
}

// TestSitesList_EmptyNormalizesToArray guarantees an absent siteEntry renders as
// [] (never null) so an agent can always range over sites.
func TestSitesList_EmptyNormalizesToArray(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /webmasters/v3/sites": {200, `{}`},
	})
	stdout := f.runOK(t, "sites", "list", "--json")
	if !strings.Contains(stdout, `"sites":[]`) {
		t.Errorf("stdout = %q, want an empty sites array", stdout)
	}
}

// TestSitesGet_URLPrefixEscaping locks the siteUrl path-segment escaping for the
// URL-prefix property form.
func TestSitesGet_URLPrefixEscaping(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F": {200, `{"siteUrl":"https://example.com/","permissionLevel":"siteOwner"}`},
	})
	stdout := f.runOK(t, "sites", "get", "--site", "https://example.com/", "--json")
	f.last(t, "GET", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F")
	if !strings.Contains(stdout, `"permissionLevel":"siteOwner"`) {
		t.Errorf("stdout = %q, want the site entry verbatim", stdout)
	}
}

// TestSitesGet_DomainPropertyEscaping locks the sc-domain: property form.
func TestSitesGet_DomainPropertyEscaping(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /webmasters/v3/sites/sc-domain%3Aexample.com": {200, `{"siteUrl":"sc-domain:example.com","permissionLevel":"siteOwner"}`},
	})
	f.runOK(t, "sites", "get", "--site", "sc-domain:example.com", "--json")
	f.last(t, "GET", "/webmasters/v3/sites/sc-domain%3Aexample.com")
}

// TestSitemapsList_JSON asserts the sitemap array is normalized to
// {"sitemaps":[...]} and both path segments are escaped.
func TestSitemapsList_JSON(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps": {200, `{"sitemap":[{"path":"https://example.com/sitemap.xml","errors":"0","warnings":"1"}]}`},
	})
	stdout := f.runOK(t, "sitemaps", "list", "--site", "https://example.com/", "--json")
	f.last(t, "GET", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps")
	var got struct {
		Sitemaps []struct {
			Path string `json:"path"`
		} `json:"sitemaps"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if len(got.Sitemaps) != 1 || got.Sitemaps[0].Path != "https://example.com/sitemap.xml" {
		t.Errorf("sitemaps = %+v, want the single normalized row", got.Sitemaps)
	}
}

// TestSitemapsGet_FeedpathEscaping locks the feedpath path-segment escaping.
func TestSitemapsGet_FeedpathEscaping(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps/https%3A%2F%2Fexample.com%2Fsitemap.xml": {200, `{"path":"https://example.com/sitemap.xml"}`},
	})
	f.runOK(t, "sitemaps", "get", "--site", "https://example.com/", "--sitemap", "https://example.com/sitemap.xml", "--json")
	f.last(t, "GET", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps/https%3A%2F%2Fexample.com%2Fsitemap.xml")
}

// TestSitemapsSubmit_PutEmptyBody asserts the write verb uses PUT, tolerates an
// empty 204 body, and reports the synthesized {"ok":true,...} envelope.
func TestSitemapsSubmit_PutEmptyBody(t *testing.T) {
	f := newFixture(t, map[string]route{
		"PUT /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps/https%3A%2F%2Fexample.com%2Fsitemap.xml": {204, ``},
	})
	stdout := f.runOK(t, "sitemaps", "submit", "--site", "https://example.com/", "--sitemap", "https://example.com/sitemap.xml", "--json")
	req := f.last(t, "PUT", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps/https%3A%2F%2Fexample.com%2Fsitemap.xml")
	if len(req.Body) != 0 {
		t.Errorf("submit body = %q, want empty (PUT with no payload)", req.Body)
	}
	var got struct {
		OK      bool   `json:"ok"`
		Site    string `json:"site"`
		Sitemap string `json:"sitemap"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if !got.OK || got.Site != "https://example.com/" || got.Sitemap != "https://example.com/sitemap.xml" {
		t.Errorf("envelope = %+v, want ok:true with echoed site/sitemap", got)
	}
}

// TestSitemapsDelete_Method asserts the delete verb uses DELETE and reports ok.
func TestSitemapsDelete_Method(t *testing.T) {
	f := newFixture(t, map[string]route{
		"DELETE /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps/https%3A%2F%2Fexample.com%2Fsitemap.xml": {204, ``},
	})
	stdout := f.runOK(t, "sitemaps", "delete", "--site", "https://example.com/", "--sitemap", "https://example.com/sitemap.xml", "--json")
	f.last(t, "DELETE", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/sitemaps/https%3A%2F%2Fexample.com%2Fsitemap.xml")
	if !strings.Contains(stdout, `"ok":true`) {
		t.Errorf("stdout = %q, want ok:true", stdout)
	}
}

// TestQuery_BodyConstruction asserts the full searchAnalytics.query body:
// dates, dimensions, type, a filter group, and the paging/state/aggregation
// knobs — all passed as native API values.
func TestQuery_BodyConstruction(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/searchAnalytics/query": {200, `{"rows":[{"keys":["pizza"],"clicks":10,"impressions":100,"ctr":0.1,"position":3.5}],"responseAggregationType":"byProperty"}`},
	})
	stdout := f.runOK(t, "query",
		"--site", "https://example.com/",
		"--start", "2026-06-01", "--end", "2026-06-30",
		"--dimensions", "query,page",
		"--type", "web",
		"--filter", "query:contains:pizza",
		"--filter", "country:equals:usa",
		"--row-limit", "10", "--start-row", "5",
		"--data-state", "all", "--aggregation", "byPage",
		"--json")

	req := f.last(t, "POST", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/searchAnalytics/query")
	var body struct {
		StartDate             string   `json:"startDate"`
		EndDate               string   `json:"endDate"`
		Dimensions            []string `json:"dimensions"`
		Type                  string   `json:"type"`
		RowLimit              int      `json:"rowLimit"`
		StartRow              int      `json:"startRow"`
		DataState             string   `json:"dataState"`
		AggregationType       string   `json:"aggregationType"`
		DimensionFilterGroups []struct {
			GroupType string `json:"groupType"`
			Filters   []struct {
				Dimension  string `json:"dimension"`
				Operator   string `json:"operator"`
				Expression string `json:"expression"`
			} `json:"filters"`
		} `json:"dimensionFilterGroups"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("request body is not JSON: %q", req.Body)
	}
	if body.StartDate != "2026-06-01" || body.EndDate != "2026-06-30" {
		t.Errorf("dates = %s..%s, want 2026-06-01..2026-06-30", body.StartDate, body.EndDate)
	}
	if len(body.Dimensions) != 2 || body.Dimensions[0] != "query" || body.Dimensions[1] != "page" {
		t.Errorf("dimensions = %v, want [query page]", body.Dimensions)
	}
	if body.Type != "web" || body.DataState != "all" || body.AggregationType != "byPage" {
		t.Errorf("type/dataState/aggregation = %s/%s/%s", body.Type, body.DataState, body.AggregationType)
	}
	if body.RowLimit != 10 || body.StartRow != 5 {
		t.Errorf("rowLimit/startRow = %d/%d, want 10/5", body.RowLimit, body.StartRow)
	}
	if len(body.DimensionFilterGroups) != 1 || body.DimensionFilterGroups[0].GroupType != "and" {
		t.Fatalf("filter groups = %+v, want one 'and' group", body.DimensionFilterGroups)
	}
	fs := body.DimensionFilterGroups[0].Filters
	if len(fs) != 2 || fs[0].Dimension != "query" || fs[0].Operator != "contains" || fs[0].Expression != "pizza" {
		t.Errorf("filters = %+v, want the two parsed filters", fs)
	}
	if fs[1].Dimension != "country" || fs[1].Operator != "equals" || fs[1].Expression != "usa" {
		t.Errorf("second filter = %+v, want country:equals:usa", fs[1])
	}
	// Query response is passed through verbatim under --json.
	if !strings.Contains(stdout, `"responseAggregationType":"byProperty"`) {
		t.Errorf("stdout = %q, want the response passed through", stdout)
	}
}

// TestQuery_DaysWindow asserts --days derives an inclusive PT window ending on
// the injected clock's date.
func TestQuery_DaysWindow(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/searchAnalytics/query": {200, `{"rows":[]}`},
	})
	f.runOK(t, "query", "--site", "https://example.com/", "--days", "28", "--json")
	req := f.last(t, "POST", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/searchAnalytics/query")
	var body struct {
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("request body is not JSON: %q", req.Body)
	}
	// fixedNow = 2026-07-21 PT; 28 inclusive days => start 2026-06-24, end 2026-07-21.
	if body.EndDate != "2026-07-21" || body.StartDate != "2026-06-24" {
		t.Errorf("window = %s..%s, want 2026-06-24..2026-07-21", body.StartDate, body.EndDate)
	}
}

// TestInspect_Body asserts the URL Inspection request body and that its result
// is passed through.
func TestInspect_Body(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /v1/urlInspection/index:inspect": {200, `{"inspectionResult":{"indexStatusResult":{"verdict":"PASS"}}}`},
	})
	stdout := f.runOK(t, "inspect",
		"--site", "https://example.com/",
		"--url", "https://example.com/page",
		"--language", "en-US",
		"--json")
	req := f.last(t, "POST", "/v1/urlInspection/index:inspect")
	if req.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want Bearer ya29.test-token", req.Auth)
	}
	var body struct {
		InspectionURL string `json:"inspectionUrl"`
		SiteURL       string `json:"siteUrl"`
		LanguageCode  string `json:"languageCode"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("request body is not JSON: %q", req.Body)
	}
	if body.InspectionURL != "https://example.com/page" || body.SiteURL != "https://example.com/" || body.LanguageCode != "en-US" {
		t.Errorf("body = %+v, want the inspect fields", body)
	}
	if !strings.Contains(stdout, `"verdict":"PASS"`) {
		t.Errorf("stdout = %q, want the inspection result passed through", stdout)
	}
}

// TestQuery_OmitsUnsetKnobs guarantees optional fields are omitted (so the API
// applies its own defaults) rather than sent as zero values.
func TestQuery_OmitsUnsetKnobs(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/searchAnalytics/query": {200, `{"rows":[]}`},
	})
	f.runOK(t, "query", "--site", "https://example.com/", "--start", "2026-06-01", "--end", "2026-06-30", "--json")
	req := f.last(t, "POST", "/webmasters/v3/sites/https%3A%2F%2Fexample.com%2F/searchAnalytics/query")
	s := string(req.Body)
	for _, k := range []string{"dimensions", "type", "rowLimit", "startRow", "dataState", "aggregationType", "dimensionFilterGroups"} {
		if strings.Contains(s, k) {
			t.Errorf("body %q should omit unset key %q", s, k)
		}
	}
}
