package calendly

import (
	"net/http"
	"strings"
	"testing"
)

func TestEventTypeListResolvesMeUser(t *testing.T) {
	routes := map[string]routeHandler{
		"/users/me":    {http.StatusOK, meResponse("BASE")},
		"/event_types": {http.StatusOK, `{"collection":[]}`},
	}
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()
	// meResponse embeds the literal "BASE"; rewrite to the live base so the URI
	// the tool builds matches what the /users/me body advertises.
	routes["/users/me"] = routeHandler{http.StatusOK, meResponse(srv.URL)}

	code, _, stderr := run(t, srv, "event-type", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	q := parseQuery(t, captured["/event_types"].Query)
	if got := q.Get("user"); got != srv.URL+"/users/ME" {
		t.Errorf("user = %q, want the resolved /users/me uri", got)
	}
	if q.Has("organization") {
		t.Errorf("organization must not be set in user scope: %q", captured["/event_types"].Query)
	}
}

func TestEventTypeListOrgScope(t *testing.T) {
	routes := map[string]routeHandler{
		"/users/me":    {http.StatusOK, meResponse("BASE")},
		"/event_types": {http.StatusOK, `{"collection":[]}`},
	}
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, routes, captured)
	defer srv.Close()
	routes["/users/me"] = routeHandler{http.StatusOK, meResponse(srv.URL)}

	code, _, stderr := run(t, srv, "event-type", "list", "--org", "--count", "10")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	q := parseQuery(t, captured["/event_types"].Query)
	if got := q.Get("organization"); got != srv.URL+"/organizations/ORG" {
		t.Errorf("organization = %q, want resolved org uri", got)
	}
	if q.Has("user") {
		t.Errorf("user must not be set in org scope")
	}
	if q.Get("count") != "10" {
		t.Errorf("count = %q, want 10", q.Get("count"))
	}
}

func TestEventTypeListExplicitUserUUIDNormalized(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"collection":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event-type", "list", "--user", "ABC123")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// No /users/me call needed; the bare UUID expands to a users/{uuid} URI.
	if got.Path != "/event_types" {
		t.Fatalf("path = %q, want /event_types (no me resolution)", got.Path)
	}
	q := parseQuery(t, got.Query)
	if want := srv.URL + "/users/ABC123"; q.Get("user") != want {
		t.Errorf("user = %q, want %q", q.Get("user"), want)
	}
}

func TestEventTypeGetExtractsUUIDFromURI(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"resource":{}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "event-type", "get", "https://api.calendly.com/event_types/ET-9")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.HasSuffix(got.Path, "/event_types/ET-9") {
		t.Errorf("path = %q, want …/event_types/ET-9", got.Path)
	}
}
