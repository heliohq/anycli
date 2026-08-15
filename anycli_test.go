package anycli

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/registry"
)

// staticResolver is a minimal CredentialResolver for wiring tests.
type staticResolver struct {
	data map[string]string
}

func (r staticResolver) Resolve(ctx context.Context, tool Tool, account string) (*Credential, error) {
	return &Credential{Data: r.data, CacheUntil: time.Now().Add(time.Hour)}, nil
}

func TestExecuteWith_UnknownTool(t *testing.T) {
	e, err := New(Config{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	exitCode, err := e.ExecuteWith(context.Background(), Tool("definitely-not-a-shipped-tool"), nil, staticResolver{}, ExecOptions{Account: "a1"})
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

func TestExecute_DelegatesToExecuteWith(t *testing.T) {
	e, err := New(Config{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	// Execute is the default-account short form: same behavior as ExecuteWith
	// with empty options for an unknown tool.
	exitCode, err := e.Execute(context.Background(), Tool("definitely-not-a-shipped-tool"), nil, staticResolver{})
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

func TestExecuteFigmaCapabilitiesThroughEmbeddedService(t *testing.T) {
	engine, err := New(Config{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	resolver := staticResolver{data: map[string]string{"access_token": "figd_test_token"}}
	exitCode, err := engine.Execute(context.Background(), Tool("figma"), []string{"capabilities"}, resolver)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

// recordingTransport records every request it sees and answers each with a
// canned response, never touching the network.
type recordingTransport struct {
	requests []*http.Request
	body     string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.requests = append(rt.requests, req)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Request:    req,
	}, nil
}

// TestConfigHTTPClientInterceptsServiceHTTP pins the Config.HTTPClient seam:
// a consumer-injected client carries every provider HTTP call a built-in
// service execution makes (the e2e upstream-mock seam).
func TestConfigHTTPClientInterceptsServiceHTTP(t *testing.T) {
	rt := &recordingTransport{body: `{"emailAddress":"fixture@example.com","messagesTotal":1,"threadsTotal":1}`}
	engine, err := New(Config{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	resolver := staticResolver{data: map[string]string{"access_token": "test-token"}}
	exitCode, err := engine.Execute(context.Background(), Tool("gmail"), []string{"profile", "--json"}, resolver)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if len(rt.requests) == 0 {
		t.Fatal("no HTTP request went through the injected client")
	}
	req := rt.requests[0]
	if req.URL.Host != "gmail.googleapis.com" {
		t.Errorf("request host = %q, want the production Gmail host (the seam must carry, not rewrite)", req.URL.Host)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want the resolver-injected token", got)
	}
}

func TestListToolsReturnsValidatedManifests(t *testing.T) {
	manifests, err := ListTools()
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("ListTools returned no tools")
	}

	for i, manifest := range manifests {
		if manifest.Name == "" || manifest.Description == "" {
			t.Errorf("manifest %d is incomplete: %+v", i, manifest)
		}
		if manifest.Title == "" || manifest.Category == "" {
			t.Errorf("manifest %q has no display title or category: %+v", manifest.Name, manifest)
		}
		if manifest.Kind != ToolKindCLI && manifest.Kind != ToolKindService {
			t.Errorf("manifest %q has invalid kind %q", manifest.Name, manifest.Kind)
		}
		if i > 0 && manifests[i-1].Name >= manifest.Name {
			t.Errorf("manifests are not strictly name-sorted: %q then %q", manifests[i-1].Name, manifest.Name)
		}
	}
}

func TestListToolsGitHubManifest(t *testing.T) {
	manifests, err := ListTools()
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	for _, manifest := range manifests {
		if manifest.Name != Tool("github") {
			continue
		}
		if manifest.Kind != ToolKindCLI {
			t.Errorf("github kind = %q, want %q", manifest.Kind, ToolKindCLI)
		}
		if !slices.Equal(manifest.CredentialFields, []string{"access_token"}) {
			t.Errorf("github credential fields = %v, want [access_token]", manifest.CredentialFields)
		}
		return
	}
	t.Fatal("github manifest not found")
}

func TestListToolsFigmaManifest(t *testing.T) {
	manifests, err := ListTools()
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	for _, manifest := range manifests {
		if manifest.Name != Tool("figma") {
			continue
		}
		if manifest.Kind != ToolKindService {
			t.Errorf("figma kind = %q, want %q", manifest.Kind, ToolKindService)
		}
		if !slices.Equal(manifest.CredentialFields, []string{"access_token"}) {
			t.Errorf("figma credential fields = %v, want [access_token]", manifest.CredentialFields)
		}
		return
	}
	t.Fatal("figma manifest not found")
}

// TestListToolsCarriesDisplayFields pins the passthrough of the two display
// fields for a handful of tools whose name and title differ in every way they
// can: an unprefixed Google surface, a name that drops the vendor's dot, and a
// vendor whose own capitalisation is lowercase.
func TestListToolsCarriesDisplayFields(t *testing.T) {
	want := map[Tool]struct{ title, category string }{
		"slack":   {"Slack", "Productivity"},
		"sheets":  {"Google Sheets", "Productivity"},
		"billcom": {"BILL", "Finance"},
		"beehiiv": {"beehiiv", "Marketing"},
		"x":       {"X", "Social & Ads"},
	}

	manifests, err := ListTools()
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	for _, manifest := range manifests {
		expected, ok := want[manifest.Name]
		if !ok {
			continue
		}
		if manifest.Title != expected.title {
			t.Errorf("%s title = %q, want %q", manifest.Name, manifest.Title, expected.title)
		}
		if manifest.Category != expected.category {
			t.Errorf("%s category = %q, want %q", manifest.Name, manifest.Category, expected.category)
		}
		delete(want, manifest.Name)
	}
	for name := range want {
		t.Errorf("%s manifest not found", name)
	}
}

// TestListToolsPostHogManifest guards the cross-repo credential-projection
// invariant: PostHog
// must expose exactly the fields the Helio provider bundle projects. The region
// host is resolved at runtime (POSTHOG_API_HOST is an environment override, not
// a host-supplied credential), so `access_token` is the only credential field.
func TestListToolsPostHogManifest(t *testing.T) {
	manifests, err := ListTools()
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	for _, manifest := range manifests {
		if manifest.Name != Tool("posthog") {
			continue
		}
		if manifest.Kind != ToolKindService {
			t.Errorf("posthog kind = %q, want %q", manifest.Kind, ToolKindService)
		}
		if !slices.Equal(manifest.CredentialFields, []string{"access_token"}) {
			t.Errorf("posthog credential fields = %v, want [access_token]", manifest.CredentialFields)
		}
		return
	}
	t.Fatal("posthog manifest not found")
}

func TestManifestForRejectsIncompleteExecutionContracts(t *testing.T) {
	cases := []struct {
		name       string
		definition *registry.Definition
		wantError  string
	}{
		{
			name:       "CLI without binary",
			definition: &registry.Definition{Name: "missing-cli"},
			wantError:  "has no binary",
		},
		{
			name:       "service without implementation",
			definition: &registry.Definition{Name: "missing-service", Type: "service"},
			wantError:  "has no registered implementation",
		},
		{
			name:       "unsupported execution kind",
			definition: &registry.Definition{Name: "unknown", Type: "remote"},
			wantError:  "unsupported type",
		},
		{
			name: "credential binding without source field",
			definition: &registry.Definition{
				Name: "figma",
				Type: "service",
				Auth: &registry.AuthConfig{Credentials: []registry.CredentialBinding{{}}},
			},
			wantError: "credential binding 0 has no source field",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manifestFor(tc.definition)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("manifestFor error = %v, want substring %q", err, tc.wantError)
			}
		})
	}
}
