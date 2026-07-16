package msgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// retryBackoffs are the delays before each automatic GET retry (Graph
// intermittently returns transient 429/5xx under rapid sequential calls).
// Length bounds the retry count. Shared by all Graph-backed tools so retry
// behaviour is identical everywhere.
var retryBackoffs = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// Client is the shared Microsoft Graph request core used by every Graph-backed
// anycli tool (microsoft-outlook, microsoft-calendar, microsoft-onedrive). It
// owns the retry policy, error/scope-hint formatting, credential
// classification, and JSON emit path so a fix to any of them lands once. The
// per-tool cobra Service embeds one (constructed from its own resolvers) and
// keeps only genuinely tool-specific bits (outlook's extra request headers,
// onedrive's raw content download and upload-session chunking).
type Client struct {
	// Provider is the tool id used in Go-side error prefixes, e.g.
	// "microsoft-outlook".
	Provider string
	// APILabel is the label for a non-2xx Graph response, e.g.
	// "microsoft-calendar API error".
	APILabel string
	// ScopeHint is appended to 401/403 errors (the usual cause is a missing
	// delegated scope the user never granted on connect).
	ScopeHint string
	// ResolveBase returns the Graph API base (no trailing slash).
	ResolveBase func() string
	// ResolveHTTP returns the HTTP client to use.
	ResolveHTTP func() *http.Client
	// ResolveOut returns the stdout writer for emit.
	ResolveOut func() io.Writer
	// Sleep overrides the retry backoff sleeper; nil = time.Sleep. Tests
	// inject a recorder to keep retries deterministic and fast.
	Sleep func(time.Duration)
	// DefaultHeaders are set on every request (calendar sends
	// Accept: application/json; outlook/onedrive send none).
	DefaultHeaders map[string]string
}

// Request describes one Graph round trip.
type Request struct {
	Method string
	// Path is base-relative (joined onto ResolveBase) or an absolute URL (a
	// Graph @odata.nextLink), and is used for error context.
	Path string
	// Endpoint, when non-empty, is the absolute URL to hit verbatim (used by
	// callers that pre-compute paging / upload endpoints); Path is then only
	// error context.
	Endpoint string
	// Query is appended to the resolved endpoint (skipped for absolute
	// nextLink paths, which carry their own state).
	Query url.Values
	// Body is the pre-marshalled request payload (nil for none).
	Body []byte
	// ContentType for Body; empty defaults to application/json when Body is
	// present.
	ContentType string
	// Headers are per-request extra headers (outlook's message fetch).
	Headers map[string]string
	// Raw marks a raw byte download (onedrive /content): an empty 2xx body is
	// a legitimate empty file, so the empty-GET retry/error guard is skipped.
	Raw bool
}

// DoJSON marshals payload as JSON (nil → no body) and performs the request.
func (c *Client) DoJSON(ctx context.Context, token, method, path string, query url.Values, payload any, headers map[string]string) ([]byte, error) {
	var body []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("%s: encode request: %w", c.Provider, err)
		}
		body = b
	}
	return c.Do(ctx, token, Request{Method: method, Path: path, Query: query, Body: body, Headers: headers})
}

// Do performs one Graph request with Bearer auth, retry, and error
// classification, returning the raw 2xx body.
//
// GET requests (idempotent) are retried up to len(retryBackoffs) times on a
// 429/5xx status or a 2xx response with an empty body where one is expected
// (non-Raw). After retries a still-empty non-Raw GET is an error. Non-GET
// requests are never auto-retried: a POST/PATCH may have applied even on a 5xx,
// and re-sending would double the side effect.
func (c *Client) Do(ctx context.Context, token string, r Request) ([]byte, error) {
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = r.Path
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = c.ResolveBase() + endpoint
		}
	}
	if len(r.Query) > 0 {
		if strings.Contains(endpoint, "?") {
			endpoint += "&" + r.Query.Encode()
		} else {
			endpoint += "?" + r.Query.Encode()
		}
	}
	for attempt := 0; ; attempt++ {
		status, body, err := c.doRequest(ctx, token, r.Method, endpoint, r.Path, r.ContentType, r.Body, r.Headers)
		if err != nil {
			return nil, err
		}
		if r.Method == http.MethodGet && attempt < len(retryBackoffs) && c.retryableGET(status, body, r.Raw) {
			c.pause(retryBackoffs[attempt])
			continue
		}
		if status < 200 || status > 299 {
			return nil, c.APIError(status, r.Path, body)
		}
		if r.Method == http.MethodGet && !r.Raw && len(bytes.TrimSpace(body)) == 0 {
			return nil, fmt.Errorf("%s: GET %s: empty response from API (HTTP %d) after %d attempts", c.Provider, r.Path, status, attempt+1)
		}
		return body, nil
	}
}

// doRequest performs a single HTTP round trip and returns status + body.
func (c *Client) doRequest(ctx context.Context, token, method, endpoint, path, contentType string, payload []byte, headers map[string]string) (int, []byte, error) {
	var reqBody io.Reader
	if len(payload) > 0 {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: build request: %w", c.Provider, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range c.DefaultHeaders {
		req.Header.Set(k, v)
	}
	if len(payload) > 0 {
		ct := contentType
		if ct == "" {
			ct = "application/json"
		}
		req.Header.Set("Content-Type", ct)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.ResolveHTTP().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %s %s: %w", c.Provider, method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: read response: %w", c.Provider, err)
	}
	return resp.StatusCode, body, nil
}

// retryableGET reports whether a GET response warrants an automatic retry:
// rate limit, server failure, or a 2xx with an empty body where one is expected
// (non-Raw).
func (c *Client) retryableGET(status int, body []byte, raw bool) bool {
	if status == http.StatusTooManyRequests || status >= 500 {
		return true
	}
	if raw {
		return false
	}
	return status >= 200 && status <= 299 && len(bytes.TrimSpace(body)) == 0
}

// pause sleeps for the retry backoff; tests inject a recorder via Sleep.
func (c *Client) pause(d time.Duration) {
	if c.Sleep != nil {
		c.Sleep(d)
		return
	}
	time.Sleep(d)
}

// APIError builds the surfaced error for a non-2xx response, appending the
// scope hint on 401/403 and classifying credential rejections.
func (c *Client) APIError(status int, path string, body []byte) error {
	hint := ""
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		hint = c.ScopeHint
	}
	apiErr := fmt.Errorf("%s (HTTP %d): %s%s", c.APILabel, status, APIMessage(body), hint)
	return ClassifyCredentialError(status, body, apiErr)
}

// Emit writes a provider JSON response to stdout. It refuses to write bytes
// that are not strictly valid JSON so --json output is always parseable. An
// empty body (e.g. a 202/204 from an action verb) is emitted as {}.
func (c *Client) Emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		return fmt.Errorf("%s: provider returned invalid JSON", c.Provider)
	}
	_, err := c.ResolveOut().Write(append(body, '\n'))
	return err
}

// EmitJSON marshals a synthesized value to stdout.
func (c *Client) EmitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%s: encode output: %w", c.Provider, err)
	}
	return c.Emit(body)
}

// APIMessage extracts Graph's error message from an error body, falling back to
// the raw body. Graph's envelope is {"error":{"code":"...","message":"..."}}.
func APIMessage(body []byte) string {
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error.Code != "" || e.Error.Message != "") {
		return strings.TrimSpace(strings.TrimPrefix(e.Error.Code+": "+e.Error.Message, ": "))
	}
	return string(body)
}
