package mixpanel

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid input. It maps to exit code 2 and
// kind "usage". (cobra-originated parse errors are treated the same way.)
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Mixpanel non-2xx response or a transport
// failure. It maps to exit code 1. kind distinguishes the sub-classes the AI
// must react to differently:
//
//   - "api"        — a generic non-2xx (the default bucket).
//   - "credential" — 401/403: the Service Account secret is wrong/expired or
//     lacks the project role. Permanent until the credential is fixed; the
//     wrapped cause is marked with execution.RejectCredential so the engine
//     invalidates it.
//   - "rateLimit"  — 429: the Query API cap (60 queries/hour, 5 concurrent) was
//     hit. Transient; retryAfter carries the Retry-After seconds when present so
//     the host can back off and retry rather than treat the call as dead.
//
// status is the HTTP status (0 for transport/network failures). It wraps the
// underlying cause so errors.As for *credentialRejectedError still resolves
// through it.
type apiError struct {
	msg        string
	status     int
	kind       string
	retryAfter int
	err        error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client carries the resolved credentials, region-selected host bases, and
// output streams for one mixpanel invocation. Commands issue requests through
// its helpers; none of them re-derive hosts or auth.
type client struct {
	hc         *http.Client
	authHeader string
	projectID  string
	queryBase  string // https://<host>/api/query
	appBase    string // https://<host>/api/app
	exportBase string // https://<exportHost>/api/2.0
	out        io.Writer
	err        io.Writer
}

// hosts holds the three region-selected API bases. Mixpanel enforces data
// residency: a US-host call against an EU/India-resident project fails, and
// this applies to every surface — Query, Export, and the App API — so host
// selection is a first-class credential input, never a constant.
type hosts struct {
	query  string
	app    string
	export string
}

// resolveHosts maps a region to its Query/App/Export bases. Empty defaults to
// US; anything outside {us, eu, in} is a config error.
func resolveHosts(region string) (hosts, error) {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "", "us":
		return hosts{
			query:  "https://mixpanel.com/api/query",
			app:    "https://mixpanel.com/api/app",
			export: "https://data.mixpanel.com/api/2.0",
		}, nil
	case "eu":
		return hosts{
			query:  "https://eu.mixpanel.com/api/query",
			app:    "https://eu.mixpanel.com/api/app",
			export: "https://data-eu.mixpanel.com/api/2.0",
		}, nil
	case "in":
		return hosts{
			query:  "https://in.mixpanel.com/api/query",
			app:    "https://in.mixpanel.com/api/app",
			export: "https://data-in.mixpanel.com/api/2.0",
		}, nil
	default:
		return hosts{}, fmt.Errorf("invalid MIXPANEL_REGION %q (must be one of us, eu, in)", region)
	}
}

// basicAuth builds the standards-compliant base64 Basic-auth header from the
// Service Account username:secret pair.
func basicAuth(username, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+secret))
}

// getJSON issues a GET against base+path with the given query values and
// returns the response body. project_id is the caller's responsibility to add
// to values where the endpoint requires it.
func (c *client) getJSON(ctx context.Context, base, path string, values url.Values) ([]byte, error) {
	return c.do(ctx, http.MethodGet, base, path, values, "", nil)
}

// postForm issues a POST against base+path where the analytical parameters
// travel in a form-encoded (application/x-www-form-urlencoded) request body,
// while query stays in the URL query string (project_id lives there even for
// POST endpoints). A nil form sends an empty body.
func (c *client) postForm(ctx context.Context, base, path string, query, form url.Values) ([]byte, error) {
	var body string
	if form != nil {
		body = form.Encode()
	}
	return c.do(ctx, http.MethodPost, base, path, query, body, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

// do performs one request and returns the body on 2xx, or a classified
// apiError otherwise.
func (c *client) do(ctx context.Context, method, base, path string, query url.Values, body string, headers map[string]string) ([]byte, error) {
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mixpanel: build request: %v", err), kind: "api", err: err}
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mixpanel: %s %s: %v", method, path, err), kind: "api", err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mixpanel: read response: %v", err), kind: "api", err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyError(resp.StatusCode, resp.Header.Get("Retry-After"), respBody)
	}
	return respBody, nil
}

// classifyError turns a non-2xx response into a typed apiError, splitting
// 401/403 (credential) and 429 (rateLimit) out of the generic bucket.
func classifyError(status int, retryAfter string, body []byte) error {
	msg := fmt.Sprintf("mixpanel API error (HTTP %d): %s", status, apiMessage(body))
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		raw := fmt.Errorf("%s (check the Service Account username/secret and that it has a role on this project)", msg)
		rejected := execution.RejectCredential(raw)
		return &apiError{msg: rejected.Error(), status: status, kind: "credential", err: rejected}
	case http.StatusTooManyRequests:
		secs := parseRetryAfter(retryAfter)
		hint := " (Mixpanel Query API limit: 60 queries/hour, 5 concurrent — back off and retry)"
		return &apiError{msg: msg + hint, status: status, kind: "rateLimit", retryAfter: secs, err: fmt.Errorf("%s", msg)}
	default:
		return &apiError{msg: msg, status: status, kind: "api", err: fmt.Errorf("%s", msg)}
	}
}

// parseRetryAfter reads a Retry-After header expressed in delta-seconds,
// returning 0 when absent or not an integer (Mixpanel returns seconds).
func parseRetryAfter(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 0
}

// apiMessage extracts a human-readable error string from a Mixpanel error
// body, falling back to the raw body. Mixpanel is inconsistent across surfaces
// (some return {"error": "..."} , some {"request":..., "error":...}), so we
// probe the common shapes then fall back.
func apiMessage(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "(empty response body)"
	}
	return s
}

func (c *client) httpClient() *http.Client {
	if c.hc != nil {
		return c.hc
	}
	return http.DefaultClient
}

// emitJSON writes a JSON response body to stdout verbatim plus a trailing
// newline (the notion/bitly convention: Mixpanel responses are already
// structured JSON, so we pass them through untouched).
func (c *client) emitJSON(body []byte) error {
	_, err := c.out.Write(append(body, '\n'))
	return err
}
