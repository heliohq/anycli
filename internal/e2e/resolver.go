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
