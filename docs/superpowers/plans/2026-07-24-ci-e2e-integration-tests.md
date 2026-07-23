# CI E2E Integration Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Real-API e2e tests in CI per design doc `docs/design/008-ci-e2e-integration-tests.md`: a token-gateway-backed `CredentialResolver` test helper, change detection that selects only affected tools, a GitHub Actions workflow (selective PR/main runs + nightly sweep with key rotation), and the first example service tests (attio).

**Architecture:** A non-build-tagged helper package `internal/e2e` implements `anycli.CredentialResolver` against Helio's `GET /connections/token` (with an env-var override for local pre-integration testing) and provides a stdout-capturing `RunTool` runner. A `go run`-able program `internal/e2e/affected` maps changed paths to tool names (definition filename == tool name, package dir == tool name with dashes stripped). Per-service tests live in `internal/tools/<pkg>/e2e_test.go` behind `//go:build e2e` so normal `go test ./...` never touches them.

**Tech Stack:** Go (stdlib only — no new dependencies), GitHub Actions, bash + curl + jq + gh for key rotation.

## Global Constraints

- All content in English (code, comments, docs, commit messages) — repo rule.
- Format with `gofmt`; no new Go module dependencies.
- Commit format `type(scope): message`; small atomic commits.
- Tests first, then implementation; run tests before marking a task complete.
- `go build ./... && go vet ./... && go test ./...` must stay green **without any e2e credentials present** — everything gateway-dependent is either unit-tested against `httptest` or behind the `e2e` build tag.
- No interactive prompts anywhere.
- Key facts from the design doc (verified against the Helio codebase):
  - Gateway: `GET <base>/connections/token?provider=<key>&account=<label>` with `Authorization: Bearer $HELIO_E2E_API_KEY`; 2xx body is `{"data": {"access_token": "...", "expires_at": "...", "credential": {...}, "account_key": "..."}}`; 404 = not connected, 409 = ambiguous account (carries candidates).
  - `HELIO_E2E_API_BASE` is the same base URL heliox uses (including any `/v1` prefix); it is **required** — the helper errors when it's unset rather than guessing a production URL.
  - Key rotation: `GET <base>/user/me` → `data.id` (the AI-user id) → `POST <base>/users/ai/<id>/api-key-refresh` → `data.secret` is the fresh key.
  - Tool naming: definition filename == tool name (enforced by `definitions.ListBundled`); service package dir == tool name with `-` removed (e.g. `adobe-sign` → `internal/tools/adobesign/`, `gate-probe` → `gateprobe`).

---

### Task 1: Provider-key mapping (`internal/e2e/provider.go`)

**Files:**
- Create: `internal/e2e/provider.go`
- Test: `internal/e2e/provider_test.go`

**Interfaces:**
- Produces: `func ProviderFor(tool string) string` (package `e2e`) — consumed by Task 2's resolver.

- [ ] **Step 1: Write the failing test**

```go
package e2e

import "testing"

func TestProviderFor(t *testing.T) {
	cases := map[string]string{
		// identity for tools whose anycli id equals the provider key
		"attio":  "attio",
		"gmail":  "gmail",
		"github": "github",
		// google short-name family
		"drive":  "google_drive",
		"sheets": "google_sheets",
		// mechanical dash↔underscore
		"adobe-sign":        "adobe_sign",
		"microsoft-outlook": "microsoft_outlook",
		// folded ids (anycli c269a6e) keep underscore provider keys
		"billcom":    "bill_com",
		"customerio": "customer_io",
		// irregular
		"search-console": "google_search_console",
	}
	for tool, want := range cases {
		if got := ProviderFor(tool); got != want {
			t.Errorf("ProviderFor(%q) = %q, want %q", tool, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/e2e/ -run TestProviderFor -v`
Expected: FAIL (package does not exist / `ProviderFor` undefined)

- [ ] **Step 3: Write the implementation**

```go
// Package e2e is the e2e test support package: a CredentialResolver backed
// by Helio's integration token gateway (design 008), an env-var override
// for local pre-integration testing, and a stdout-capturing tool runner.
//
// The package carries no build tag so it is compiled and unit-tested by the
// normal `go test ./...` run; only the per-service e2e_test.go files (which
// hit real provider APIs) are behind the `e2e` build tag.
package e2e

// toolToProvider maps anycli tool names to Helio provider catalog keys
// where the two differ. Identity holds for every other tool. This is a
// copy of helio-cli/internal/toolcred.toolToProvider (not importable:
// internal package of another module), updated for anycli's current tool
// ids (bill-com→billcom, customer-io→customerio were folded by c269a6e).
// Keep in sync with that table; the source of truth for the provider keys
// is Helio's provider catalog.
var toolToProvider = map[string]string{
	"adobe-sign":         "adobe_sign",
	"billcom":            "bill_com",
	"customerio":         "customer_io",
	"dropbox-sign":       "dropbox_sign",
	"facebook-pages":     "facebook_pages",
	"google-ads":         "google_ads",
	"google-analytics":   "google_analytics",
	"help-scout":         "help_scout",
	"lemon-squeezy":      "lemon_squeezy",
	"meta-ads":           "meta_ads",
	"microsoft-calendar": "microsoft_calendar",
	"microsoft-onedrive": "microsoft_onedrive",
	"microsoft-outlook":  "microsoft_outlook",
	"search-console":     "google_search_console",
	"sprout-social":      "sprout_social",
	"zoho-books":         "zoho_books",
	"zoho-crm":           "zoho_crm",
	// Google short-name family (design 303 on the Helio side).
	"calendar": "google_calendar",
	"contacts": "google_contacts",
	"docs":     "google_docs",
	"drive":    "google_drive",
	"forms":    "google_forms",
	"meet":     "google_meet",
	"sheets":   "google_sheets",
	"slides":   "google_slides",
	"tasks":    "google_tasks",
}

// ProviderFor returns the Helio provider catalog key for an anycli tool.
func ProviderFor(tool string) string {
	if p, ok := toolToProvider[tool]; ok {
		return p
	}
	return tool
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/e2e/ -run TestProviderFor -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/e2e/provider.go internal/e2e/provider_test.go
git commit -m "feat(e2e): add tool-to-provider key mapping for the token gateway"
```

---

### Task 2: Gateway-backed resolver with env override (`internal/e2e/resolver.go`)

**Files:**
- Create: `internal/e2e/resolver.go`
- Test: `internal/e2e/resolver_test.go`

**Interfaces:**
- Consumes: `ProviderFor` (Task 1); `anycli.Credential{Data map[string]string, CacheUntil time.Time}`, `anycli.CredentialResolver` (existing public API, see `anycli.go` and `cmd/anycli/resolver.go` for the shape).
- Produces (consumed by Task 3's runner):
  - `func NewResolver() (*Resolver, error)` — builds from `HELIO_E2E_API_KEY` + `HELIO_E2E_API_BASE`; error mentions the missing variable.
  - `(*Resolver).Resolve(ctx, tool anycli.Tool, account string) (*anycli.Credential, error)` — satisfies `anycli.CredentialResolver`.
  - `type NotConnectedError struct{ Tool, Account string }` and `func IsNotConnected(err error) bool`.
  - `func credentialFromEnv(account string) map[string]string` — reads the `ANYCLI_E2E_CRED_<ACCOUNT>_<FIELD>` override (account `""` reads `PRIMARY`).

- [ ] **Step 1: Write the failing tests**

```go
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveFromGateway(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections/token" {
			t.Errorf("path = %q, want /connections/token", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-e2e-test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("provider"); got != "google_drive" {
			t.Errorf("provider = %q, want google_drive (mapped from drive)", got)
		}
		if got := r.URL.Query().Get("account"); got != "secondary" {
			t.Errorf("account = %q, want secondary", got)
		}
		exp := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"access_token": "tok-123",
			"expires_at":   exp,
			"credential":   map[string]string{"access_token": "tok-123", "subject": "a@b.c"},
		}})
	}))
	defer srv.Close()

	t.Setenv("HELIO_E2E_API_KEY", "sk-e2e-test")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, err := NewResolver()
	if err != nil {
		t.Fatal(err)
	}
	cred, err := r.Resolve(context.Background(), "drive", "secondary")
	if err != nil {
		t.Fatal(err)
	}
	if cred.Data["access_token"] != "tok-123" || cred.Data["subject"] != "a@b.c" {
		t.Errorf("Data = %v", cred.Data)
	}
	if !cred.CacheUntil.After(time.Now()) || !cred.CacheUntil.Before(time.Now().Add(30*time.Minute)) {
		t.Errorf("CacheUntil = %v, want inside (now, expires_at) with safety margin", cred.CacheUntil)
	}
}

func TestResolveAccessTokenOnlyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"access_token": "tok-9"}})
	}))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, _ := NewResolver()
	cred, err := r.Resolve(context.Background(), "attio", "")
	if err != nil {
		t.Fatal(err)
	}
	// Empty credential map falls back to {"access_token": ...}, and a
	// response without expires_at gets the default 50-minute horizon.
	if cred.Data["access_token"] != "tok-9" {
		t.Errorf("Data = %v", cred.Data)
	}
}

func TestResolveNotConnectedIs404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"not_found"}}`, http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, _ := NewResolver()
	_, err := r.Resolve(context.Background(), "attio", "")
	if !IsNotConnected(err) {
		t.Fatalf("err = %v, want NotConnectedError", err)
	}
}

func TestResolveOtherHTTPErrorsAreNotSkips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"unauthorized"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	r, _ := NewResolver()
	_, err := r.Resolve(context.Background(), "attio", "")
	if err == nil || IsNotConnected(err) {
		t.Fatalf("err = %v, want a hard (non-skip) error", err)
	}
}

func TestEnvOverrideBeatsGateway(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	t.Setenv("HELIO_E2E_API_KEY", "k")
	t.Setenv("HELIO_E2E_API_BASE", srv.URL)
	t.Setenv("ANYCLI_E2E_CRED_PRIMARY_ACCESS_TOKEN", "local-tok")
	t.Setenv("ANYCLI_E2E_CRED_SECONDARY_ACCESS_TOKEN", "local-tok-2")
	r, _ := NewResolver()

	cred, err := r.Resolve(context.Background(), "attio", "")
	if err != nil || cred.Data["access_token"] != "local-tok" {
		t.Fatalf("default account: cred=%v err=%v", cred, err)
	}
	cred, err = r.Resolve(context.Background(), "attio", "secondary")
	if err != nil || cred.Data["access_token"] != "local-tok-2" {
		t.Fatalf("secondary account: cred=%v err=%v", cred, err)
	}
	if called {
		t.Fatal("gateway must not be called when the env override is set")
	}
}

func TestNewResolverRequiresConfig(t *testing.T) {
	t.Setenv("HELIO_E2E_API_KEY", "")
	t.Setenv("HELIO_E2E_API_BASE", "")
	if _, err := NewResolver(); err == nil {
		t.Fatal("want error when HELIO_E2E_API_KEY / HELIO_E2E_API_BASE are unset")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/e2e/ -v`
Expected: FAIL (`NewResolver`, `Resolver`, `IsNotConnected` undefined)

- [ ] **Step 3: Write the implementation**

```go
package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	anycli "github.com/heliohq/anycli"
)

const (
	envAPIKey  = "HELIO_E2E_API_KEY"
	envAPIBase = "HELIO_E2E_API_BASE"
	// credEnvPrefix marks local-override credential fields:
	// ANYCLI_E2E_CRED_PRIMARY_ACCESS_TOKEN -> account "", field "access_token";
	// ANYCLI_E2E_CRED_SECONDARY_X -> account "secondary", field "x".
	// This is the design-008 D9 bridge: a new tool's author runs the same
	// e2e cases locally with a hand-held token before the provider exists
	// in Helio. CI never sets these variables.
	credEnvPrefix = "ANYCLI_E2E_CRED_"

	// defaultTokenTTL mirrors toolcred: cache a token the gateway returned
	// without an expiry for 50 minutes.
	defaultTokenTTL = 50 * time.Minute
	// expirySafetyMargin backs the cache horizon off the real expiry so an
	// in-flight call never rides a token that lapses mid-request.
	expirySafetyMargin = 60 * time.Second
)

// NotConnectedError reports that the e2e assistant has no active connection
// for (tool, account). Tests skip on it (design 008 D1: skipped, not failed).
type NotConnectedError struct {
	Tool    string
	Account string
}

func (e *NotConnectedError) Error() string {
	return fmt.Sprintf("no e2e connection for tool %q account %q", e.Tool, e.Account)
}

// IsNotConnected reports whether err is a NotConnectedError.
func IsNotConnected(err error) bool {
	var nc *NotConnectedError
	return errors.As(err, &nc)
}

// tokenResponse mirrors integration-service's dto.TokenResponse (the fields
// e2e consumes).
type tokenResponse struct {
	AccessToken string            `json:"access_token"`
	ExpiresAt   *time.Time        `json:"expires_at"`
	Credential  map[string]string `json:"credential"`
}

// Resolver implements anycli.CredentialResolver against Helio's integration
// token gateway (GET /connections/token), the same contract heliox's
// internal/toolcred uses. The engine's own Cache handles (tool, account)
// caching via CacheUntil, so the resolver holds no cache of its own.
type Resolver struct {
	base string
	key  string
	hc   *http.Client
}

// NewResolver builds a Resolver from HELIO_E2E_API_KEY and
// HELIO_E2E_API_BASE. Both are required; the base must be the same API base
// heliox uses (including any /v1 prefix).
func NewResolver() (*Resolver, error) {
	key := os.Getenv(envAPIKey)
	base := strings.TrimRight(os.Getenv(envAPIBase), "/")
	if key == "" {
		return nil, fmt.Errorf("%s is not set", envAPIKey)
	}
	if base == "" {
		return nil, fmt.Errorf("%s is not set", envAPIBase)
	}
	return &Resolver{base: base, key: key, hc: &http.Client{Timeout: 30 * time.Second}}, nil
}

// Resolve returns the credential for (tool, account). The local env
// override, when present for the account, wins over the gateway.
func (r *Resolver) Resolve(ctx context.Context, tool anycli.Tool, account string) (*anycli.Credential, error) {
	if data := credentialFromEnv(account); len(data) > 0 {
		return &anycli.Credential{Data: data, CacheUntil: time.Now().Add(defaultTokenTTL)}, nil
	}

	q := url.Values{}
	q.Set("provider", ProviderFor(string(tool)))
	if account != "" {
		q.Set("account", account)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+"/connections/token?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Accept", "application/json")

	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token gateway: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, &NotConnectedError{Tool: string(tool), Account: account}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		// 401 (dead key) and 409 (ambiguous account) are hard failures:
		// they mean broken e2e configuration, never "skip quietly".
		return nil, fmt.Errorf("token gateway: HTTP %d for tool %q account %q", resp.StatusCode, tool, account)
	}

	var env struct {
		Data tokenResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("token gateway: decode response: %w", err)
	}

	data := env.Data.Credential
	if len(data) == 0 {
		if env.Data.AccessToken == "" {
			return nil, fmt.Errorf("token gateway: empty credential for tool %q", tool)
		}
		data = map[string]string{"access_token": env.Data.AccessToken}
	}
	cacheUntil := time.Now().Add(defaultTokenTTL)
	if env.Data.ExpiresAt != nil {
		cacheUntil = env.Data.ExpiresAt.Add(-expirySafetyMargin)
	}
	return &anycli.Credential{Data: data, CacheUntil: cacheUntil}, nil
}

// credentialFromEnv reads the ANYCLI_E2E_CRED_<ACCOUNT>_<FIELD> override for
// one account. account "" selects PRIMARY. Returns nil when no variable for
// the account is set.
func credentialFromEnv(account string) map[string]string {
	name := account
	if name == "" {
		name = "primary"
	}
	prefix := credEnvPrefix + strings.ToUpper(name) + "_"
	data := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || v == "" || !strings.HasPrefix(k, prefix) {
			continue
		}
		field := strings.ToLower(strings.TrimPrefix(k, prefix))
		if field != "" {
			data[field] = v
		}
	}
	if len(data) == 0 {
		return nil
	}
	return data
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/e2e/ -v`
Expected: PASS (all six tests)

- [ ] **Step 5: Run the full offline suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS with no e2e env vars set

- [ ] **Step 6: Commit**

```bash
git add internal/e2e/resolver.go internal/e2e/resolver_test.go
git commit -m "feat(e2e): gateway-backed credential resolver with local env override"
```

---

### Task 3: `RunTool` runner and run-id prefix (`internal/e2e/run.go`)

**Files:**
- Create: `internal/e2e/run.go`
- Test: `internal/e2e/run_test.go`

**Interfaces:**
- Consumes: `NewResolver`, `IsNotConnected` (Task 2); `anycli.New(anycli.Config{})`, `(*anycli.Engine).ExecuteWith(ctx, tool, args, resolver, anycli.ExecOptions{Account: account})` (existing public API).
- Produces (consumed by per-service e2e tests, Task 6):
  - `func RunTool(t *testing.T, tool, account string, args ...string) (stdout string, exitCode int)` — skips the test when credentials are absent or the tool is not connected; fails the test on engine/config errors; returns tool-level failures as a nonzero exit code (callers assert).
  - `func Prefix() string` — `"anycli-e2e-<runid>-"`, stable within one process.

- [ ] **Step 1: Write the failing tests**

```go
package e2e

import (
	"strings"
	"testing"
)

func TestPrefixIsStableAndTagged(t *testing.T) {
	p1, p2 := Prefix(), Prefix()
	if p1 != p2 {
		t.Errorf("Prefix not stable: %q vs %q", p1, p2)
	}
	if !strings.HasPrefix(p1, "anycli-e2e-") || !strings.HasSuffix(p1, "-") {
		t.Errorf("Prefix = %q, want anycli-e2e-<runid>-", p1)
	}
}

func TestCaptureStdout(t *testing.T) {
	out, err := captureStdout(func() error {
		_, werr := osStdoutWriteString("hello e2e")
		return werr
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello e2e" {
		t.Errorf("captured %q", out)
	}
}

func TestRunToolSkipsWithoutCredentials(t *testing.T) {
	t.Setenv("HELIO_E2E_API_KEY", "")
	t.Setenv("HELIO_E2E_API_BASE", "")
	res := testing.RunTests(func(pat, str string) (bool, error) { return true, nil },
		[]testing.InternalTest{{Name: "probe", F: func(st *testing.T) {
			RunTool(st, "attio", "", "whoami")
		}}})
	// The inner test must NOT fail — it must skip. RunTests returns true
	// when nothing failed (skips count as ok).
	if !res {
		t.Fatal("RunTool must skip, not fail, when no e2e credentials are configured")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/e2e/ -run 'TestPrefix|TestCapture|TestRunToolSkips' -v`
Expected: FAIL (`Prefix`, `captureStdout`, `osStdoutWriteString`, `RunTool` undefined)

- [ ] **Step 3: Write the implementation**

```go
package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	anycli "github.com/heliohq/anycli"
)

var (
	engineOnce sync.Once
	engine     *anycli.Engine
	engineErr  error

	// stdoutMu serializes stdout capture: built-in services print their
	// JSON to os.Stdout, so concurrent RunTool calls in one process would
	// interleave. Closed-loop e2e chains are sequential anyway.
	stdoutMu sync.Mutex

	prefixOnce sync.Once
	prefix     string
)

// Prefix returns the run-scoped test-data prefix "anycli-e2e-<runid>-"
// (design 008 D4): GITHUB_RUN_ID in CI, a timestamp locally. All data an
// e2e test creates must carry it so interrupted-run leftovers are
// identifiable by the nightly sweep.
func Prefix() string {
	prefixOnce.Do(func() {
		id := os.Getenv("GITHUB_RUN_ID")
		if id == "" {
			id = fmt.Sprintf("%d", time.Now().Unix())
		}
		prefix = "anycli-e2e-" + id + "-"
	})
	return prefix
}

// RunTool executes one tool invocation through the real engine with the e2e
// resolver and returns its captured stdout and exit code.
//
// Skip semantics (design 008): missing e2e configuration or a not-connected
// tool skips the test (t.Skip) with an "E2E-PENDING" marker the workflow
// greps into the job summary. Engine-level errors fail the test. A nonzero
// exit code is returned, not fatal — closed-loop tests assert on it (e.g.
// "get after delete must fail").
func RunTool(t *testing.T, tool, account string, args ...string) (string, int) {
	t.Helper()

	resolver, err := NewResolver()
	if err != nil {
		if credentialFromEnv(account) != nil {
			// Local override present: run with a resolver that only
			// serves env credentials (gateway config not required).
			resolver = &Resolver{}
		} else {
			t.Skipf("E2E-PENDING tool=%s: %v", tool, err)
		}
	}

	engineOnce.Do(func() {
		engine, engineErr = anycli.New(anycli.Config{})
	})
	if engineErr != nil {
		t.Fatalf("anycli.New: %v", engineErr)
	}

	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	var exit int
	var execErr error
	out, capErr := captureStdout(func() error {
		exit, execErr = engine.ExecuteWith(context.Background(), anycli.Tool(tool), args, resolver,
			anycli.ExecOptions{Account: account})
		return nil
	})
	if capErr != nil {
		t.Fatalf("capture stdout: %v", capErr)
	}
	if execErr != nil {
		if IsNotConnected(execErr) {
			t.Skipf("E2E-PENDING tool=%s account=%q: %v", tool, account, execErr)
		}
		// API-level failures also surface as an error next to a nonzero
		// exit; log it and let the caller assert on the exit code.
		t.Logf("tool %s exit=%d err: %v", tool, exit, execErr)
	}
	return out, exit
}

// captureStdout redirects os.Stdout around fn and returns what fn printed.
func captureStdout(fn func() error) (string, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fnErr := fn()
	w.Close()
	os.Stdout = old
	out := <-done
	return out, fnErr
}

// osStdoutWriteString exists for the capture unit test: it writes through
// the (possibly redirected) os.Stdout at call time.
func osStdoutWriteString(s string) (int, error) {
	return os.Stdout.WriteString(s)
}
```

Note on `RunTool`'s missing-config path: when `NewResolver` fails but a local override exists, a zero-value `&Resolver{}` is safe — `Resolve` checks `credentialFromEnv` before touching `r.base`/`r.key`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/e2e/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/e2e/run.go internal/e2e/run_test.go
git commit -m "feat(e2e): stdout-capturing RunTool runner with pending-skip semantics"
```

---

### Task 4: Change detection (`internal/e2e/affected`)

**Files:**
- Create: `internal/e2e/affected/affected.go` (library logic)
- Create: `internal/e2e/affected/main.go` (`go run` entry)
- Test: `internal/e2e/affected/affected_test.go`

**Interfaces:**
- Consumes: `definitions.ListBundled() ([]*registry.Definition, error)` — each has `.Name` and `.Type` (`"service"` or `""`/cli).
- Produces:
  - `func PkgDir(tool string) string` — tool name with `-` removed.
  - `func Affected(changed []string, tools []string) (affected []string, smoke bool)` — pure path classification.
  - CLI contract (used by the workflow, Task 5): `go run ./internal/e2e/affected -base <ref>` and `go run ./internal/e2e/affected -all`, both printing a JSON array of `{"tool": "...", "level": "warn"|"required"}` objects to stdout (skip-level tools filtered out, announced on stderr).
  - `var SmokeTools = []string{"attio", "github", "hunter", "billcom", "gmail"}`
  - `type Level string` with `LevelSkip`/`LevelWarn`/`LevelRequired`, `func PolicyFor(tool string) Level` (design 008 D8; unlisted tools default to `LevelWarn`).

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/heliohq/anycli/definitions"
)

var testTools = []string{"attio", "adobe-sign", "gate-probe", "github", "hunter", "billcom", "gmail"}

func TestAffectedByDefinitionFile(t *testing.T) {
	got, smoke := Affected([]string{"definitions/tools/attio.json"}, testTools)
	if smoke || !reflect.DeepEqual(got, []string{"attio"}) {
		t.Errorf("got %v smoke=%v", got, smoke)
	}
}

func TestAffectedByServicePackageWithDashDivergence(t *testing.T) {
	got, _ := Affected([]string{"internal/tools/adobesign/client.go"}, testTools)
	if !reflect.DeepEqual(got, []string{"adobe-sign"}) {
		t.Errorf("got %v, want [adobe-sign] (pkg dir has no dash)", got)
	}
}

func TestSharedCodeTriggersSmoke(t *testing.T) {
	for _, p := range []string{
		"anycli.go", "go.mod", "internal/exec/exec.go", "internal/credential/inject.go",
		"internal/middleware/engine.go", "internal/registry/schema.go",
		"internal/config/dirs.go", "definitions/embed.go",
		"internal/tools/register.go", "internal/tools/registry.go",
		"internal/e2e/resolver.go",
	} {
		if _, smoke := Affected([]string{p}, testTools); !smoke {
			t.Errorf("path %q must trigger the smoke subset", p)
		}
	}
}

func TestDocsAndWorkflowChangesAreIgnored(t *testing.T) {
	got, smoke := Affected([]string{"docs/design/008-x.md", "README.md", ".github/workflows/ci.yml"}, testTools)
	if len(got) != 0 || smoke {
		t.Errorf("got %v smoke=%v, want none", got, smoke)
	}
}

func TestAffectedDeduplicatesAndSorts(t *testing.T) {
	got, _ := Affected([]string{
		"internal/tools/attio/records.go",
		"definitions/tools/attio.json",
		"definitions/tools/hunter.json",
	}, testTools)
	if !reflect.DeepEqual(got, []string{"attio", "hunter"}) {
		t.Errorf("got %v", got)
	}
}

// Naming lint (design 008 D5): every bundled service definition must map to
// an existing internal/tools/<PkgDir(name)> directory via the strip-dash
// rule. This is what makes manifest-free path mapping safe.
func TestEveryServiceDefinitionHasMatchingPackageDir(t *testing.T) {
	defs, err := definitions.ListBundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.Type != "service" {
			continue
		}
		dir := "../../tools/" + PkgDir(d.Name)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Errorf("service definition %q has no package dir internal/tools/%s", d.Name, PkgDir(d.Name))
		}
	}
}

func TestPolicyDefaultsToWarn(t *testing.T) {
	if got := PolicyFor("some-unlisted-tool"); got != LevelWarn {
		t.Errorf("PolicyFor(unlisted) = %q, want warn", got)
	}
}

func TestPolicyTableOnlyContainsBundledTools(t *testing.T) {
	defs, err := definitions.ListBundled()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, d := range defs {
		byName[d.Name] = true
	}
	for tool := range policy {
		if !byName[tool] {
			t.Errorf("policy table entry %q has no bundled definition", tool)
		}
	}
}

func TestMatrixEntriesFilterSkipAndCarryLevel(t *testing.T) {
	// matrixEntries is the shared output shaping used by both -base and
	// -all modes.
	old := policy
	policy = map[string]Level{"hunter": LevelSkip, "attio": LevelRequired}
	defer func() { policy = old }()

	got := matrixEntries([]string{"attio", "hunter", "gmail"})
	want := []MatrixEntry{
		{Tool: "attio", Level: LevelRequired},
		{Tool: "gmail", Level: LevelWarn},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSmokeToolsAreBundled(t *testing.T) {
	defs, err := definitions.ListBundled()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, d := range defs {
		byName[d.Name] = true
	}
	for _, s := range SmokeTools {
		if !byName[s] {
			t.Errorf("smoke tool %q has no bundled definition", s)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/e2e/affected/ -v`
Expected: FAIL (package does not exist)

- [ ] **Step 3: Write the library implementation**

```go
// affected maps changed file paths to the e2e tool matrix (design 008 D5).
// There is no checked-in manifest: definition filename == tool name
// (enforced by definitions.ListBundled) and service package dir == tool
// name with dashes stripped (enforced by a lint test here). The package is
// `main` (together with main.go) so `go run ./internal/e2e/affected` works.
package main

import (
	"sort"
	"strings"
)

// SmokeTools is the fixed representative subset run when shared code
// changes: service + cli tool types, single- and multi-field credential
// shapes, and one Google-family tool. Tools without e2e tests or without a
// connection simply skip (design 008 D1/D9).
var SmokeTools = []string{"attio", "github", "hunter", "billcom", "gmail"}

// sharedPrefixes are the paths whose change affects every tool: the engine
// pipeline, credential handling, registry/schema, the embed loader, the
// service registry files directly under internal/tools/, and the e2e
// helper itself.
var sharedPrefixes = []string{
	"anycli.go",
	"go.mod",
	"go.sum",
	"definitions/embed",
	"internal/exec/",
	"internal/credential/",
	"internal/middleware/",
	"internal/registry/",
	"internal/config/",
	"internal/e2e/",
	"cmd/",
}

// Level is a tool's e2e blocking policy (design 008 D8).
type Level string

const (
	LevelSkip     Level = "skip"     // explicitly silenced, filtered from the matrix
	LevelWarn     Level = "warn"     // runs, failure visible, does not block merge
	LevelRequired Level = "required" // failure fails the e2e-gate job (branch protection)
)

// policy assigns non-default levels. Every unlisted tool is warn. Promote a
// tool to required after a proven stable streak; demote to skip when its
// provider is known-broken (design 008 D8: a per-tool graduation path, not
// a global switch).
var policy = map[string]Level{
	// No required tools yet — the table starts warn-only by design.
	// Example promotions/demotions:
	//   "stripe": LevelRequired,
	//   "hunter": LevelSkip, // provider maintenance until 2026-08-01
}

// PolicyFor returns the blocking level for a tool; unlisted tools are warn.
func PolicyFor(tool string) Level {
	if l, ok := policy[tool]; ok {
		return l
	}
	return LevelWarn
}

// MatrixEntry is one element of the JSON matrix the workflow consumes.
type MatrixEntry struct {
	Tool  string `json:"tool"`
	Level Level  `json:"level"`
}

// matrixEntries shapes a tool list into matrix entries: skip-level tools
// are filtered out (announced on stderr by main), the rest carry their
// level so the workflow can set continue-on-error per job.
func matrixEntries(tools []string) []MatrixEntry {
	out := []MatrixEntry{}
	for _, tool := range tools {
		if PolicyFor(tool) == LevelSkip {
			continue
		}
		out = append(out, MatrixEntry{Tool: tool, Level: PolicyFor(tool)})
	}
	return out
}

// PkgDir returns the internal/tools package directory name for a tool:
// the tool name with dashes removed (adobe-sign -> adobesign).
func PkgDir(tool string) string {
	return strings.ReplaceAll(tool, "-", "")
}

// Affected classifies changed paths against the known tool list. It returns
// the sorted, deduplicated affected tool names and whether shared code
// changed (caller substitutes SmokeTools).
func Affected(changed []string, tools []string) ([]string, bool) {
	byPkg := make(map[string]string, len(tools))
	for _, tool := range tools {
		byPkg[PkgDir(tool)] = tool
	}
	byName := make(map[string]bool, len(tools))
	for _, tool := range tools {
		byName[tool] = true
	}

	hit := map[string]bool{}
	smoke := false
	for _, p := range changed {
		switch {
		case strings.HasPrefix(p, "definitions/tools/") && strings.HasSuffix(p, ".json"):
			name := strings.TrimSuffix(strings.TrimPrefix(p, "definitions/tools/"), ".json")
			if byName[name] {
				hit[name] = true
			}
		case strings.HasPrefix(p, "internal/tools/"):
			rest := strings.TrimPrefix(p, "internal/tools/")
			dir, _, isSub := strings.Cut(rest, "/")
			if isSub {
				if tool, ok := byPkg[dir]; ok {
					hit[tool] = true
					continue
				}
			}
			// Files directly under internal/tools/ (register.go,
			// registry.go, lint_test.go) are shared plumbing.
			smoke = true
		default:
			for _, prefix := range sharedPrefixes {
				if strings.HasPrefix(p, prefix) {
					smoke = true
					break
				}
			}
		}
	}

	out := make([]string, 0, len(hit))
	for tool := range hit {
		out = append(out, tool)
	}
	sort.Strings(out)
	return out, smoke
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/e2e/affected/ -v`
Expected: PASS

- [ ] **Step 5: Write the CLI entry**

```go
package main

// This file is the `go run ./internal/e2e/affected` entry the e2e workflow
// calls. Modes:
//
//	-base <ref>   diff HEAD against <ref>, print affected tools as JSON
//	-all          print every tool that has an e2e_test.go, as JSON
//
// On any git failure the program falls back to the smoke subset and says so
// on stderr — a broken diff must degrade to "run something", never to
// "silently run nothing" (design 008: no silent caps).

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/heliohq/anycli/definitions"
)

func main() {
	base := flag.String("base", "", "git ref to diff HEAD against")
	all := flag.Bool("all", false, "list every tool that has e2e tests")
	flag.Parse()

	defs, err := definitions.ListBundled()
	if err != nil {
		fatal(err)
	}
	var tools []string
	for _, d := range defs {
		tools = append(tools, d.Name)
	}

	var result []string
	switch {
	case *all:
		result = toolsWithE2ETests(tools)
	case *base != "":
		changed, err := gitDiff(*base)
		if err != nil {
			fmt.Fprintf(os.Stderr, "affected: git diff failed (%v); falling back to smoke subset\n", err)
			result = SmokeTools
			break
		}
		var smoke bool
		result, smoke = Affected(changed, tools)
		if smoke {
			result = mergeSorted(result, SmokeTools)
		}
	default:
		fatal(fmt.Errorf("one of -base or -all is required"))
	}

	for _, tool := range result {
		if PolicyFor(tool) == LevelSkip {
			fmt.Fprintf(os.Stderr, "affected: %s is policy-skipped (design 008 D8)\n", tool)
		}
	}
	out, err := json.Marshal(matrixEntries(result))
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func gitDiff(base string) ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", base+"...HEAD").Output()
	if err != nil {
		// Fall back to a two-dot diff for shallow/force-push cases where
		// the merge base is unavailable.
		out, err = exec.Command("git", "diff", "--name-only", base, "HEAD").Output()
		if err != nil {
			return nil, err
		}
	}
	return strings.Fields(string(out)), nil
}

func toolsWithE2ETests(tools []string) []string {
	var out []string
	for _, tool := range tools {
		if _, err := os.Stat("internal/tools/" + PkgDir(tool) + "/e2e_test.go"); err == nil {
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func mergeSorted(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "affected:", err)
	os.Exit(1)
}
```

Note: `affected.go`, `main.go`, and `affected_test.go` are all `package main` so `go run ./internal/e2e/affected` works; the package is not importable API, so this costs nothing.

- [ ] **Step 6: Run everything, including a live smoke of the CLI**

Run: `go test ./internal/e2e/... -v && go run ./internal/e2e/affected -base HEAD~1 && go run ./internal/e2e/affected -all`
Expected: tests PASS; first `go run` prints a JSON array of `{"tool":...,"level":...}` objects (likely `[]` or the smoke set at level warn); second prints `[]` (no e2e tests exist yet)

- [ ] **Step 7: Commit**

```bash
git add internal/e2e/affected/
git commit -m "feat(e2e): affected-tool detection with strip-dash mapping and smoke fallback"
```

---

### Task 5: E2E workflow + key-rotation script

**Files:**
- Create: `.github/workflows/e2e.yml`
- Create: `.github/scripts/rotate-key.sh`

**Interfaces:**
- Consumes: `go run ./internal/e2e/affected` CLI contract (Task 4); `E2E-PENDING` skip marker (Task 3); repository secrets `HELIO_E2E_API_KEY`, `E2E_SECRETS_PAT` and repository variable `HELIO_E2E_API_BASE` (documented in Task 7's runbook).
- Produces: the CI surface — selective e2e on same-repo PRs and main pushes, nightly full sweep + pending report + key rotation, and the `e2e-gate` aggregator job. Branch protection registers **only `e2e-gate`** (never individual matrix jobs); it fails exactly when a required-level tool fails (design 008 D8).

- [ ] **Step 1: Write the workflow**

```yaml
name: E2E

# Real-API e2e per design 008. Non-blocking by design (D8): this workflow
# must NOT be added to required status checks.

on:
  push:
    branches:
      - main
  pull_request:
  schedule:
    - cron: "17 3 * * *"
  workflow_dispatch:

permissions:
  contents: read

jobs:
  detect:
    runs-on: ubuntu-latest
    # Fork PRs get no secrets (design 008 D6) — skip the whole workflow.
    if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository
    outputs:
      tools: ${{ steps.detect.outputs.tools }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - id: detect
        run: |
          case "${{ github.event_name }}" in
            schedule|workflow_dispatch)
              TOOLS=$(go run ./internal/e2e/affected -all) ;;
            pull_request)
              TOOLS=$(go run ./internal/e2e/affected -base "${{ github.event.pull_request.base.sha }}") ;;
            *)
              TOOLS=$(go run ./internal/e2e/affected -base "${{ github.event.before }}") ;;
          esac
          echo "tools=$TOOLS" >> "$GITHUB_OUTPUT"
          echo "affected tools: $TOOLS"

  e2e:
    needs: detect
    if: needs.detect.outputs.tools != '[]'
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        include: ${{ fromJSON(needs.detect.outputs.tools) }}
    # warn-level failures stay visible (red matrix job) but do not fail the
    # workflow; only required-level failures propagate to e2e-gate (D8).
    continue-on-error: ${{ matrix.level == 'warn' }}
    # Per-tool serialization across PR / main / nightly runs (design 008 D7).
    # Never cancel a running job: closed-loop tests must finish their cleanup.
    concurrency:
      group: e2e-${{ matrix.tool }}
      cancel-in-progress: false
    env:
      HELIO_E2E_API_KEY: ${{ secrets.HELIO_E2E_API_KEY }}
      HELIO_E2E_API_BASE: ${{ vars.HELIO_E2E_API_BASE }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Run e2e tests
        run: |
          PKG=$(echo "${{ matrix.tool }}" | tr -d '-')
          go test -tags e2e -v -run 'TestE2E' "./internal/tools/${PKG}/..." 2>&1 | tee e2e.log
          exit "${PIPESTATUS[0]}"
      - name: Report pending (not-connected) tools
        if: always()
        run: |
          if grep -h "E2E-PENDING" e2e.log > pending.txt 2>/dev/null && [ -s pending.txt ]; then
            {
              echo "### E2E pending — has tests but no gateway connection"
              sed 's/^/- /' pending.txt
            } >> "$GITHUB_STEP_SUMMARY"
          fi

  # The ONLY e2e check allowed in branch protection (design 008 D8). warn
  # failures are absorbed by continue-on-error above, so any propagated
  # matrix failure is by construction a required-level one.
  e2e-gate:
    needs: [detect, e2e]
    if: always()
    runs-on: ubuntu-latest
    steps:
      - name: Gate on required-level failures
        run: |
          if [ "${{ needs.e2e.result }}" = "failure" ]; then
            echo "a required-level tool's e2e failed"
            exit 1
          fi
          echo "no required-level failures (e2e result: ${{ needs.e2e.result }})"

  rotate-key:
    # Nightly only. Runs even when e2e failed: a missed rotation is 24h
    # from a dead key (design 008 D6).
    needs: [e2e]
    if: always() && github.event_name == 'schedule'
    runs-on: ubuntu-latest
    concurrency:
      group: e2e-rotate-key
      cancel-in-progress: false
    env:
      HELIO_E2E_API_KEY: ${{ secrets.HELIO_E2E_API_KEY }}
      HELIO_E2E_API_BASE: ${{ vars.HELIO_E2E_API_BASE }}
      GH_TOKEN: ${{ secrets.E2E_SECRETS_PAT }}
    steps:
      - uses: actions/checkout@v4
      - name: Rotate HELIO_E2E_API_KEY
        run: ./.github/scripts/rotate-key.sh
```

- [ ] **Step 2: Write the rotation script**

```bash
#!/usr/bin/env bash
# Rotate HELIO_E2E_API_KEY (design 008 D1): exchange the current e2e
# assistant key for a fresh 48h key via the runtime self-renewal endpoint,
# verify the new key actually works, then write it back to the repository
# secret. Fails loudly on every step — a silent rotation failure means a
# dead key within 24h.
set -euo pipefail

: "${HELIO_E2E_API_KEY:?HELIO_E2E_API_KEY is required}"
: "${HELIO_E2E_API_BASE:?HELIO_E2E_API_BASE is required}"
: "${GH_TOKEN:?GH_TOKEN (PAT with secrets:write) is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

base="${HELIO_E2E_API_BASE%/}"

# 1. Who am I? The key's subject is the e2e assistant's AI user.
me=$(curl -fsS -H "Authorization: Bearer ${HELIO_E2E_API_KEY}" "${base}/user/me")
ai_user_id=$(jq -re '.data.id' <<<"$me")
echo "rotating key for AI user ${ai_user_id}"

# 2. Self-renew: mints a fresh 48h key; the old key expires naturally.
refreshed=$(curl -fsS -X POST -H "Authorization: Bearer ${HELIO_E2E_API_KEY}" \
  "${base}/users/ai/${ai_user_id}/api-key-refresh")
new_key=$(jq -re '.data.secret' <<<"$refreshed")

# 3. Verify the fresh key BEFORE persisting it. Never overwrite a working
#    secret with a broken one.
curl -fsS -H "Authorization: Bearer ${new_key}" "${base}/user/me" > /dev/null
echo "fresh key verified"

# 4. Persist.
gh secret set HELIO_E2E_API_KEY --repo "${GITHUB_REPOSITORY}" --body "${new_key}"
echo "HELIO_E2E_API_KEY rotated"
```

- [ ] **Step 3: Make the script executable and lint what's lintable**

Run:
```bash
chmod +x .github/scripts/rotate-key.sh
bash -n .github/scripts/rotate-key.sh
command -v shellcheck > /dev/null && shellcheck .github/scripts/rotate-key.sh || echo "shellcheck not installed, skipped"
command -v actionlint > /dev/null && actionlint .github/workflows/e2e.yml || echo "actionlint not installed, skipped"
```
Expected: `bash -n` exits 0; linters pass or report "not installed, skipped"

- [ ] **Step 4: Sanity-check the detect logic end to end locally**

Run: `go run ./internal/e2e/affected -base HEAD~1`
Expected: a JSON array on stdout (content depends on the last commit)

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/e2e.yml .github/scripts/rotate-key.sh
git commit -m "ci(e2e): selective e2e workflow with nightly sweep and key rotation"
```

---

### Task 6: First example service e2e tests (attio)

**Files:**
- Create: `internal/tools/attio/e2e_test.go`

**Interfaces:**
- Consumes: `e2e.RunTool`, `e2e.Prefix` (Task 3). Attio's command surface (see `internal/tools/attio/attio.go`, `records.go`, `meta.go`): `whoami`, `record create <object> --values <json>`, `record get <object> <record_id>`, `record delete <object> <record_id>`; all commands accept `--json`.
- Produces: the reference pattern every later service copies — a read smoke test plus a closed-loop write chain (design 008 D4).

- [ ] **Step 1: Confirm the exact wire shape for `--values`**

Read `internal/tools/attio/records_test.go` and `records.go` to confirm how `--values` JSON maps onto the Attio API (Attio record values are attribute-keyed; the plain-object form below must match what the existing unit tests fixture). Adjust the `--values` literal in Step 2 to the shape the client actually sends — the chain structure (create → get → delete → verify-gone) stays identical.

- [ ] **Step 2: Write the e2e tests**

```go
//go:build e2e

// Real-API e2e for the attio service (design 008 D4): a read smoke plus a
// closed-loop record chain. The chain is self-cleaning — the delete step IS
// the cleanup; the anycli-e2e-<runid>- name prefix makes any interrupted-run
// leftovers identifiable.
package attio_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

func TestE2EWhoami(t *testing.T) {
	out, exit := e2e.RunTool(t, "attio", "", "whoami", "--json")
	if exit != 0 {
		t.Fatalf("whoami exit = %d, output:\n%s", exit, out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("whoami output is not JSON: %v\n%s", err, out)
	}
	if len(v) == 0 {
		t.Fatal("whoami returned empty JSON")
	}
}

func TestE2ERecordClosedLoop(t *testing.T) {
	name := e2e.Prefix() + "company"

	// Create.
	out, exit := e2e.RunTool(t, "attio", "", "record", "create", "companies",
		"--values", fmt.Sprintf(`{"name":%q}`, name), "--json")
	if exit != 0 {
		t.Fatalf("create exit = %d, output:\n%s", exit, out)
	}
	recordID := extractRecordID(t, out)

	// Read back: the created name must be visible.
	out, exit = e2e.RunTool(t, "attio", "", "record", "get", "companies", recordID, "--json")
	if exit != 0 {
		t.Fatalf("get exit = %d, output:\n%s", exit, out)
	}
	if !strings.Contains(out, name) {
		t.Fatalf("get output does not contain created name %q:\n%s", name, out)
	}

	// Delete (this IS the cleanup).
	out, exit = e2e.RunTool(t, "attio", "", "record", "delete", "companies", recordID, "--json")
	if exit != 0 {
		t.Fatalf("delete exit = %d, output:\n%s", exit, out)
	}

	// Verify gone: get after delete must fail.
	out, exit = e2e.RunTool(t, "attio", "", "record", "get", "companies", recordID, "--json")
	if exit == 0 {
		t.Fatalf("get after delete succeeded, record %s still exists:\n%s", recordID, out)
	}
}

// extractRecordID pulls the record id out of a create/get JSON response.
// Attio nests it as id.record_id; fall back to a top-level record_id.
func extractRecordID(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		ID struct {
			RecordID string `json:"record_id"`
		} `json:"id"`
		RecordID string `json:"record_id"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse create output: %v\n%s", err, out)
	}
	if v.ID.RecordID != "" {
		return v.ID.RecordID
	}
	if v.RecordID != "" {
		return v.RecordID
	}
	t.Fatalf("no record id in output:\n%s", out)
	return ""
}
```

If Step 1 revealed a different response envelope (e.g. the service prints `{"data": {...}}`), adjust `extractRecordID` to match — the unit-test fixtures in `records_test.go` show the exact printed shape.

- [ ] **Step 3: Verify the build-tag isolation**

Run: `go build ./... && go vet ./... && go test ./internal/tools/attio/`
Expected: PASS — the e2e file is invisible without `-tags e2e`

Run: `go vet -tags e2e ./internal/tools/attio/`
Expected: PASS — the e2e file compiles under the tag

- [ ] **Step 4: Verify skip behavior without credentials**

Run: `go test -tags e2e -run TestE2E ./internal/tools/attio/ -v`
Expected: both tests SKIP with `E2E-PENDING tool=attio: HELIO_E2E_API_KEY is not set`

- [ ] **Step 5: Run against the real API (requires credentials)**

If a real Attio token is available (from the e2e assistant's connection or a dev token), run:
```bash
ANYCLI_E2E_CRED_PRIMARY_ACCESS_TOKEN=<real-token> go test -tags e2e -run TestE2E ./internal/tools/attio/ -v
```
Expected: PASS (both tests green against the real API). If no token is available in this environment, note that explicitly in the task report — do NOT mark this step done.

- [ ] **Step 6: Verify -all detection now finds attio**

Run: `go run ./internal/e2e/affected -all`
Expected: `[{"tool":"attio","level":"warn"}]`

- [ ] **Step 7: Commit**

```bash
git add internal/tools/attio/e2e_test.go
git commit -m "test(attio): first real-API e2e — whoami smoke and record closed loop"
```

---

### Task 7: Makefile target + runbook

**Files:**
- Modify: `Makefile` (append target; match the existing target style in the file)
- Create: `docs/e2e.md`

**Interfaces:**
- Consumes: everything above.
- Produces: `make e2e TOOL=<name>` and the operator runbook for bootstrap.

- [ ] **Step 1: Add the Makefile target**

Append to `Makefile` (adjust to the file's existing tab/phony conventions):

```makefile
.PHONY: e2e
# Run one tool's e2e tests: make e2e TOOL=attio
# Credentials come from HELIO_E2E_API_KEY + HELIO_E2E_API_BASE (gateway) or
# ANYCLI_E2E_CRED_* (local override) — see docs/e2e.md.
e2e:
	@test -n "$(TOOL)" || (echo "usage: make e2e TOOL=<tool-name>" && exit 1)
	go test -tags e2e -v -run 'TestE2E' ./internal/tools/$(shell echo $(TOOL) | tr -d '-')/...
```

- [ ] **Step 2: Verify the target**

Run: `make e2e TOOL=attio`
Expected: tests run and SKIP (no credentials in this shell) — proving wiring, tag, and dash-stripping work

Run: `make e2e`
Expected: `usage: make e2e TOOL=<tool-name>` and exit 1

- [ ] **Step 3: Write the runbook**

Create `docs/e2e.md`:

```markdown
# E2E Tests: Operations Runbook

Design: [design 008](design/008-ci-e2e-integration-tests.md).

## How it works

- Per-service tests live in `internal/tools/<pkg>/e2e_test.go` behind the
  `e2e` build tag; the normal test suite never runs them.
- Credentials come from Helio's integration token gateway: the e2e helper
  (`internal/e2e`) calls `GET /connections/token` as the **e2e assistant**,
  using `HELIO_E2E_API_KEY`. Nothing provider-specific is stored in this
  repository.
- CI (`.github/workflows/e2e.yml`) runs only the tools affected by a change
  (same-repo PRs and pushes to `main`), plus a nightly full sweep.
- Blocking is per tool (design 008 D8): the policy table in
  `internal/e2e/affected/affected.go` assigns `skip` / `warn` (default) /
  `required`. warn failures are visible but never block; required failures
  fail the `e2e-gate` job. Branch protection may register **only
  `e2e-gate`** — never individual matrix jobs. Promote a tool to required
  after a stable streak; demote to skip when its provider is known-broken
  (leave a dated comment saying why).

## One-time bootstrap (human)

1. Create a dedicated Helio org + assistant for e2e. Test accounts only —
   never production data.
2. Connect each service's test account(s) through the normal Helio connect
   flow and grant them to the e2e assistant. For counterpart-account tests
   (e.g. gmail), connect two accounts and note their account labels.
3. Obtain the assistant's current `HELIO_API_KEY` (a Clerk `ak_*` key from
   its runtime). Set repository secret `HELIO_E2E_API_KEY` to it.
4. Set repository variable `HELIO_E2E_API_BASE` to the API base heliox uses
   (including any `/v1` prefix).
5. Create a fine-grained PAT with `secrets: write` on this repository; set
   it as repository secret `E2E_SECRETS_PAT`.

The key self-rotates nightly (48h TTL, refreshed every 24h by the
`rotate-key` job). If CI is down for more than 48h the chain breaks —
repeat step 3.

## Running locally

Against the gateway (tools already connected in Helio):

    HELIO_E2E_API_KEY=ak_... HELIO_E2E_API_BASE=https://... make e2e TOOL=attio

With a hand-held token, before the provider exists in Helio (design 008 D9):

    ANYCLI_E2E_CRED_PRIMARY_ACCESS_TOKEN=<token> make e2e TOOL=attio

Multi-field credentials use one variable per field
(`ANYCLI_E2E_CRED_PRIMARY_<FIELD>`); counterpart accounts use the account
label (`ANYCLI_E2E_CRED_SECONDARY_<FIELD>`, selected by passing account
"secondary" to `e2e.RunTool`).

## Adding e2e tests for a service

1. Create `internal/tools/<pkg>/e2e_test.go` with `//go:build e2e`, package
   `<pkg>_test`. Copy the pattern from `internal/tools/attio/e2e_test.go`:
   one read smoke test, then closed-loop write chains (create → verify →
   delete → verify gone). Name every created object with `e2e.Prefix()`.
2. Verify locally with a real token (env override above) before merging.
3. After the provider is connected in Helio, the nightly sweep picks the
   tool up automatically. Until then its tests skip and appear in the
   nightly "E2E pending" summary.
```

- [ ] **Step 4: Run the full offline suite one last time**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add Makefile docs/e2e.md
git commit -m "docs(e2e): make target and operations runbook"
```

---

## Deviations From the Design Doc (intentional, small)

- `HELIO_E2E_API_BASE` is **required** rather than defaulting to the production API base: hardcoding a production URL in the library would rot; the workflow supplies it as a repository variable.
- No checked-in manifest file: definition-name == filename is already enforced by `definitions.ListBundled`, and the strip-dash package rule is enforced by `TestEveryServiceDefinitionHasMatchingPackageDir` — the "manifest" is these two tested invariants (the design allowed "a manifest or go generate'd table"; a tested derivation rule is the degenerate table).
- Nightly leftover sweep-cleanup (design D6) is **not implemented in this round**: it needs per-service "list + delete by prefix" logic that only makes sense once several services have e2e tests. The closed-loop chains self-clean; the prefix convention is in place. Tracked as follow-up work.
```
