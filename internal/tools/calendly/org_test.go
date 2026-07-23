package calendly

import (
	"net/http"
	"testing"
)

func TestOrgMembersResolvesOrgAndFilters(t *testing.T) {
	routes := map[string]routeHandler{
		"/users/me":                 {http.StatusOK, meResponse("BASE")},
		"/organization_memberships": {http.StatusOK, `{"collection":[]}`},
	}
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()
	routes["/users/me"] = routeHandler{http.StatusOK, meResponse(srv.URL)}

	code, _, stderr := run(t, srv, "org", "members", "--email", "team@x.com")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	q := parseQuery(t, captured["/organization_memberships"].Query)
	if q.Get("organization") != srv.URL+"/organizations/ORG" {
		t.Errorf("organization = %q, want resolved org uri", q.Get("organization"))
	}
	if q.Get("email") != "team@x.com" {
		t.Errorf("email = %q, want team@x.com", q.Get("email"))
	}
}
