package front

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

const maxErrorBodyBytes = 8 << 10

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, or a bad enum value. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Front non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejected sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Front API request with Bearer auth. query is optional; a
// nil/empty payload sends no body. A non-2xx surfaces the body's error message
// (with an actionable hint for auth/permission/rate-limit failures) as an
// apiError carrying the HTTP status; a transport failure is an apiError with
// status 0. The returned body may be empty (Front 204 no-content mutations).
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("front: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("front: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("front: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("front: read response: %v", err), err: err}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, body, token)
	}
	return body, nil
}

// newAPIError builds an apiError from a non-2xx Front response, redacting the
// token, capping the body, and classifying a 401 as a credential rejection so
// the CLI surfaces reconnect guidance.
func newAPIError(status int, body []byte, token string) error {
	msg := apiMessage(body)
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[REDACTED]")
	}
	if len(msg) > maxErrorBodyBytes {
		msg = msg[:maxErrorBodyBytes] + "…"
	}
	hint := ""
	switch status {
	case http.StatusUnauthorized:
		hint = " (access token is invalid or expired — reconnect Front)"
	case http.StatusForbidden:
		hint = " (token may lack the required scope or company permission)"
	case http.StatusTooManyRequests:
		hint = " (rate limit exceeded — retry after the provider reset window)"
	}
	raw := fmt.Errorf("front API error (HTTP %d %s): %s%s", status, http.StatusText(status), msg, hint)
	if status == http.StatusUnauthorized {
		return &apiError{msg: raw.Error(), status: status, err: execution.RejectCredential(raw)}
	}
	return &apiError{msg: raw.Error(), status: status, err: raw}
}

// apiMessage extracts Front's error message. Front error bodies are
// {"_error":{"status":<int>,"title":"…","message":"…"}}; fall back to the raw
// body when it does not match.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			Title   string `json:"title"`
			Message string `json:"message"`
		} `json:"_error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error.Title != "" || e.Error.Message != "") {
		if e.Error.Title != "" && e.Error.Message != "" {
			return e.Error.Title + ": " + e.Error.Message
		}
		return e.Error.Title + e.Error.Message
	}
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "empty response body"
	}
	return raw
}

// emitObject writes a single Front resource under the provider-neutral
// {"data":{…}} envelope. A raw provider object goes in verbatim as data.
func (s *Service) emitObject(body []byte) error {
	raw := json.RawMessage(body)
	if len(bytes.TrimSpace(body)) == 0 {
		raw = json.RawMessage("null")
	}
	return s.emitValue(map[string]any{"data": raw})
}

// emitOK writes the no-content success envelope for 204 mutations
// (update/assign/tag), where Front returns no body.
func (s *Service) emitOK() error {
	return s.emitValue(map[string]any{"data": map[string]any{"ok": true}})
}

// frontList is Front's list envelope: _results carries the page items and
// _pagination.next is the absolute URL of the next page (null at the end).
type frontList struct {
	Results    json.RawMessage `json:"_results"`
	Pagination struct {
		Next *string `json:"next"`
	} `json:"_pagination"`
}

// emitList normalizes a Front list response into
// {"data":[…],"next_page_token":"<cursor|empty>"}. The opaque cursor is the
// page_token query param lifted out of the absolute _pagination.next URL, so
// the agent never sees or handles a Front URL.
func (s *Service) emitList(body []byte) error {
	var env frontList
	if err := json.Unmarshal(body, &env); err != nil {
		return &apiError{msg: fmt.Sprintf("front: decode list response: %v", err), err: err}
	}
	data := env.Results
	if len(data) == 0 {
		data = json.RawMessage("[]")
	}
	return s.emitValue(map[string]any{
		"data":            data,
		"next_page_token": nextPageToken(env.Pagination.Next),
	})
}

// nextPageToken lifts the opaque page_token out of a Front _pagination.next URL.
// A nil/empty next (end of results) yields "". A URL that unexpectedly carries
// no page_token yields "" as well — the agent stops paginating rather than
// looping on a stale cursor.
func nextPageToken(next *string) string {
	if next == nil || *next == "" {
		return ""
	}
	u, err := url.Parse(*next)
	if err != nil {
		return ""
	}
	return u.Query().Get("page_token")
}

// emitValue marshals v and writes it to stdout with a trailing newline.
func (s *Service) emitValue(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("front: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(b); err != nil {
		return err
	}
	_, err = io.WriteString(s.stdout(), "\n")
	return err
}

// pageQuery builds the shared list query params: limit (>0) and page_token
// (non-empty). Callers add resource-specific params to the returned Values.
func pageQuery(limit int, pageToken string) url.Values {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	return q
}
