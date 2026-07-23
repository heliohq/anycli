package googleanalytics

import (
	"net/http"
	"strings"
	"testing"
)

const accountSummariesBody = `{
  "accountSummaries": [
    {
      "name": "accountSummaries/100",
      "account": "accounts/100",
      "displayName": "Acme Inc",
      "propertySummaries": [
        {"property": "properties/123456", "displayName": "acme.com", "propertyType": "PROPERTY_TYPE_ORDINARY"},
        {"property": "properties/654321", "displayName": "shop.acme.com", "propertyType": "PROPERTY_TYPE_ORDINARY"}
      ]
    },
    {
      "name": "accountSummaries/200",
      "account": "accounts/200",
      "displayName": "Side Project",
      "propertySummaries": []
    }
  ],
  "nextPageToken": "tok-2"
}`

func TestPropertyList(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {status: http.StatusOK, body: accountSummariesBody},
	})
	stdout := f.runOK(t, "property", "list")

	req := f.last(t, "GET", "/admin/v1beta/accountSummaries")
	if req.Auth != "Bearer ya29.test-token" {
		t.Errorf("Authorization = %q, want the injected bearer token", req.Auth)
	}
	if req.Query != "" {
		t.Errorf("query = %q, want empty without pagination flags", req.Query)
	}
	for _, want := range []string{"properties/123456", "acme.com", "properties/654321", "Acme Inc"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}
	if !strings.Contains(stdout, "next page token: tok-2") {
		t.Errorf("stdout = %q, want the next-page-token trailer", stdout)
	}
}

func TestPropertyListPaginationFlags(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {status: http.StatusOK, body: `{"accountSummaries":[]}`},
	})
	f.runOK(t, "property", "list", "--page-size", "50", "--page-token", "tok-1")

	req := f.last(t, "GET", "/admin/v1beta/accountSummaries")
	if !strings.Contains(req.Query, "pageSize=50") || !strings.Contains(req.Query, "pageToken=tok-1") {
		t.Errorf("query = %q, want pageSize=50 and pageToken=tok-1", req.Query)
	}
}

func TestPropertyListJSONPassthrough(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {status: http.StatusOK, body: `{"accountSummaries":[]}`},
	})
	stdout := f.runOK(t, "property", "list", "--json")
	if strings.TrimSpace(stdout) != `{"accountSummaries":[]}` {
		t.Errorf("stdout = %q, want the provider body verbatim", stdout)
	}
}

func TestPropertyListEmpty(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /admin/v1beta/accountSummaries": {status: http.StatusOK, body: `{}`},
	})
	stdout := f.runOK(t, "property", "list")
	if !strings.Contains(stdout, "no properties") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}
