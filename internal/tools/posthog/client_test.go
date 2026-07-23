package posthog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// forbiddenServer fails the test if it is ever hit — used to prove a probe was
// skipped.
func forbiddenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to forbidden host: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	}))
}

func TestRegionProbeFallsThroughUSToEU(t *testing.T) {
	usHits, euHits := capturedRequest{}, capturedRequest{}
	us := singleRouteServer(t, http.StatusUnauthorized, `{"type":"authentication_error","detail":"Invalid token"}`, &usHits)
	defer us.Close()
	eu := singleRouteServer(t, http.StatusOK, `{"email":"ada@eu.example"}`, &euHits)
	defer eu.Close()

	svc := &Service{usHost: us.URL, euHost: eu.URL}
	exit, stdout, stderr := runService(t, svc, map[string]string{EnvAccessToken: testToken}, "whoami")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if usHits.Path != "/api/users/@me" {
		t.Fatalf("US region was not probed: %+v", usHits)
	}
	if euHits.Path != "/api/users/@me" {
		t.Fatalf("EU region was not probed after US 401: %+v", euHits)
	}
	if !strings.Contains(stdout, "ada@eu.example") {
		t.Fatalf("stdout = %q, want EU user body", stdout)
	}
	if !strings.Contains(stderr, eu.URL) {
		t.Fatalf("stderr = %q, want resolved EU host", stderr)
	}
}

func TestRegionProbeCachesResolvedHost(t *testing.T) {
	euHits := 0
	us := singleRouteServer(t, http.StatusUnauthorized, `{"detail":"nope"}`, &capturedRequest{})
	defer us.Close()
	eu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		euHits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
	}))
	defer eu.Close()

	svc := &Service{usHost: us.URL, euHost: eu.URL, HC: http.DefaultClient}
	// First call resolves EU; second call must reuse the cached host and NOT
	// re-probe US (the US server would 401 and break correctness if it did).
	if _, _, _ = runService(t, svc, map[string]string{EnvAccessToken: testToken}, "whoami"); svc.region != eu.URL {
		t.Fatalf("region cache = %q, want %q", svc.region, eu.URL)
	}
	exit, _, stderr := runService(t, svc, map[string]string{EnvAccessToken: testToken}, "project", "list")
	if exit != 0 {
		t.Fatalf("second call exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if euHits < 2 {
		t.Fatalf("EU hits = %d, want >=2 (both calls served from the cached region)", euHits)
	}
}

func TestRegionProbeBothUnauthorizedRejectsCredential(t *testing.T) {
	us := singleRouteServer(t, http.StatusUnauthorized, `{"detail":"nope"}`, &capturedRequest{})
	defer us.Close()
	eu := singleRouteServer(t, http.StatusUnauthorized, `{"detail":"nope"}`, &capturedRequest{})
	defer eu.Close()

	svc := &Service{usHost: us.URL, euHost: eu.URL}
	assertCredentialRejected(t, svc, map[string]string{EnvAccessToken: testToken}, "whoami")
}

func TestAPIHostOverrideSkipsProbe(t *testing.T) {
	got := capturedRequest{}
	host := singleRouteServer(t, http.StatusOK, `{"email":"self@hosted"}`, &got)
	defer host.Close()
	forbidden := forbiddenServer(t)
	defer forbidden.Close()

	svc := &Service{usHost: forbidden.URL, euHost: forbidden.URL}
	env := map[string]string{EnvAccessToken: testToken, EnvAPIHost: host.URL}
	exit, stdout, _ := runService(t, svc, env, "whoami")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Path != "/api/users/@me" {
		t.Fatalf("POSTHOG_API_HOST was not used: %+v", got)
	}
	if !strings.Contains(stdout, "self@hosted") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestBaseURLDisablesProbe(t *testing.T) {
	got := capturedRequest{}
	base := singleRouteServer(t, http.StatusOK, `{"count":0,"results":[]}`, &got)
	defer base.Close()
	forbidden := forbiddenServer(t)
	defer forbidden.Close()

	svc := &Service{BaseURL: base.URL, usHost: forbidden.URL, euHost: forbidden.URL, HC: base.Client()}
	exit, _, stderr := runService(t, svc, map[string]string{EnvAccessToken: testToken}, "project", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if got.Path != "/api/projects/" {
		t.Fatalf("request path = %q, want /api/projects/", got.Path)
	}
}

func TestUnauthorizedOnRealCallRejectsCredential(t *testing.T) {
	srv := singleRouteServer(t, http.StatusUnauthorized, `{"type":"authentication_error","detail":"Invalid personal API key."}`, &capturedRequest{})
	defer srv.Close()
	svc := &Service{BaseURL: srv.URL, HC: srv.Client()}
	assertCredentialRejected(t, svc, map[string]string{EnvAccessToken: testToken}, "project", "list")
}

func TestRateLimitPassesThroughAsRuntimeError(t *testing.T) {
	srv := singleRouteServer(t, http.StatusTooManyRequests, `{"type":"throttled_error","detail":"Request was throttled. Expected available in 60 seconds."}`, &capturedRequest{})
	defer srv.Close()
	exit, stdout, stderr := run(t, srv, "project", "list")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 for 429", exit)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on error", stdout)
	}
	if !strings.Contains(stderr, "throttled") {
		t.Fatalf("stderr = %q, want 429 body passthrough", stderr)
	}
}
