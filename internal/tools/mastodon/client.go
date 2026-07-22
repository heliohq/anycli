package mastodon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or an unresolvable id. It maps
// to exit code 2 and code "usage_error".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Mastodon non-2xx response, a transport
// failure, or a local media/poll timeout. It maps to exit code 1 and code
// "api_error". status is the HTTP status (0 for transport/network failures);
// providerError is Mastodon's own error text when present. It wraps the
// underlying cause so errors.As for the credential-rejection marker still
// resolves through it.
type apiError struct {
	msg           string
	status        int
	providerError string
	err           error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// normalizeInstanceURL reduces a user-supplied instance URL to scheme+host with
// no trailing slash. A bare host (no scheme) is assumed https. Returns "" when
// the value has no usable host.
func normalizeInstanceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
}

// baseURL returns the API host: the test override when set, otherwise the
// instance URL derived from the credential.
func (rt *runContext) baseURL() string {
	if rt.svc.BaseURL != "" {
		return strings.TrimRight(rt.svc.BaseURL, "/")
	}
	return rt.instanceURL
}

// call performs one Mastodon API request with the bearer token and returns the
// raw body plus the response headers (Link header carries pagination cursors).
// path is instance-relative and must start with "/". A non-2xx surfaces
// Mastodon's error message as an apiError carrying the HTTP status; a transport
// failure surfaces as an apiError with status 0.
func (rt *runContext) call(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, http.Header, error) {
	target := rt.baseURL() + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+rt.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return rt.do(req)
}

// do sends a prepared request (auth already set) and classifies the response.
// Media upload builds its own multipart request and reuses this tail.
func (rt *runContext) do(req *http.Request) ([]byte, http.Header, error) {
	hc := rt.svc.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: %s %s: %v", req.Method, req.URL.Path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		provider := apiMessage(body)
		raw := &apiError{
			msg:           fmt.Sprintf("mastodon API error (HTTP %d): %s", resp.StatusCode, provider),
			status:        resp.StatusCode,
			providerError: provider,
		}
		return nil, resp.Header, classifyCredentialError(resp.StatusCode, raw)
	}
	return body, resp.Header, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so
// the engine can invalidate the stored token. A 403 is a scope/permission
// problem (the token is valid but lacks a scope) and must NOT reject it.
func classifyCredentialError(status int, err *apiError) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts Mastodon's error text. Mastodon returns
// {"error":"…"} and sometimes {"error":"…","error_description":"…"}; the
// description is appended when present. Falls back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error != "" {
		if e.Description != "" && e.Description != e.Error {
			return e.Error + ": " + e.Description
		}
		return e.Error
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(no response body)"
	}
	return trimmed
}

// parseLinkCursor extracts the max_id (next, older) cursor from an RFC 5988
// Link response header. Mastodon paginates by opaque max_id/min_id in the Link
// header rather than by offset; the AI paginates forward by passing the
// returned cursor back as --cursor (mapped to max_id). Returns "" when there is
// no next page.
func parseLinkCursor(header http.Header) string {
	link := header.Get("Link")
	if link == "" {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		open := strings.IndexByte(part, '<')
		close := strings.IndexByte(part, '>')
		if open < 0 || close <= open {
			continue
		}
		u, err := url.Parse(part[open+1 : close])
		if err != nil {
			continue
		}
		if maxID := u.Query().Get("max_id"); maxID != "" {
			return maxID
		}
	}
	return ""
}

// emitJSON marshals v and writes it to stdout followed by a newline.
func (rt *runContext) emitJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("mastodon: encode output: %v", err), err: err}
	}
	_, err = rt.svc.stdout().Write(append(b, '\n'))
	return err
}
