package docs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// retryBackoffs are the delays before each automatic GET retry (transient
// 429/5xx under rapid sequential calls). Length bounds the retry count.
var retryBackoffs = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// scopeHint is appended to 401 / scope-insufficient 403 errors: the token
// lacks a scope the user never granted, so a reconnect is required.
const scopeHint = " (possibly missing scope — disconnect and reconnect to grant access)"

// shareHint is appended to 404 / permission 403 errors: the connected account
// cannot open the document. This is the highest-frequency Docs failure — the
// user pastes a link shared with account A while the assistant is connected as
// account B.
const shareHint = " (the connected account may not have access to this document — confirm it is shared with the connected account, or use --account to select a different one)"

// docURLPattern extracts the documentId from a full Docs URL
// (https://docs.google.com/document/d/<id>/edit).
var docURLPattern = regexp.MustCompile(`/document/d/([a-zA-Z0-9_-]+)`)

// extractDocumentID accepts either a bare documentId or a full Docs URL and
// returns the id. Pasting a link is the highest-frequency input shape, so both
// are first-class.
func extractDocumentID(arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", fmt.Errorf("docs: empty document id")
	}
	if m := docURLPattern.FindStringSubmatch(arg); m != nil {
		return m[1], nil
	}
	if strings.Contains(arg, "/") || strings.Contains(arg, "docs.google.com") {
		return "", fmt.Errorf("docs: could not extract a documentId from %q — pass a bare id or a https://docs.google.com/document/d/<id>/... URL", arg)
	}
	return arg, nil
}

// docURL builds the canonical edit URL for a documentId.
func docURL(id string) string {
	return "https://docs.google.com/document/d/" + id + "/edit"
}

// call performs one Docs API request with Bearer auth. Non-2xx surfaces the
// body's error message with a reconnect or sharing hint as appropriate.
//
// GET requests (idempotent) are retried up to len(retryBackoffs) times on a
// 429/5xx status. Non-GET requests are never auto-retried: a POST may have been
// applied even when the response is a 5xx, and re-sending would double the side
// effect.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := s.base() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payloadBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("docs: encode request: %w", err)
		}
		payloadBytes = b
	}
	for attempt := 0; ; attempt++ {
		status, body, err := s.doRequest(ctx, token, method, endpoint, path, payloadBytes)
		if err != nil {
			return nil, err
		}
		if method == http.MethodGet && attempt < len(retryBackoffs) && (status == http.StatusTooManyRequests || status >= 500) {
			s.pause(retryBackoffs[attempt])
			continue
		}
		if status < 200 || status > 299 {
			apiErr := fmt.Errorf("docs API error (HTTP %d): %s%s", status, apiMessage(body), hintFor(status, body))
			return nil, classifyCredentialError(status, body, apiErr)
		}
		return body, nil
	}
}

// doRequest performs a single HTTP round trip and returns status + body.
func (s *Service) doRequest(ctx context.Context, token, method, endpoint, path string, payload []byte) (int, []byte, error) {
	var reqBody io.Reader
	if len(payload) > 0 {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("docs: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("docs: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("docs: read response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// pause sleeps for the retry backoff; tests inject a recorder via s.sleep.
func (s *Service) pause(d time.Duration) {
	if s.sleep != nil {
		s.sleep(d)
		return
	}
	time.Sleep(d)
}

// emit writes a provider JSON response to stdout, refusing bytes that are not
// strictly valid JSON so --json output is always parseable.
func (s *Service) emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return fmt.Errorf("docs: provider returned invalid JSON")
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("docs: encode output: %w", err)
	}
	return s.emit(body)
}

// apiError is the Google JSON error envelope, covering both the modern
// details[].reason (ErrorInfo) form and the legacy errors[].reason form.
type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Status  string `json:"status"`
		Message string `json:"message"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	} `json:"error"`
}

// parseAPIError decodes the error envelope; ok is false when the body is not a
// recognizable Google error.
func parseAPIError(body []byte) (apiError, bool) {
	var e apiError
	if err := json.Unmarshal(body, &e); err != nil {
		return apiError{}, false
	}
	if e.Error.Status == "" && e.Error.Message == "" && e.Error.Code == 0 {
		return apiError{}, false
	}
	return e, true
}

// reason returns the first non-empty error reason (details take precedence).
func (e apiError) reason() string {
	for _, d := range e.Error.Details {
		if d.Reason != "" {
			return d.Reason
		}
	}
	for _, d := range e.Error.Errors {
		if d.Reason != "" {
			return d.Reason
		}
	}
	return ""
}

// apiMessage extracts Google's error message, falling back to the raw body.
func apiMessage(body []byte) string {
	if e, ok := parseAPIError(body); ok {
		return strings.TrimSpace(strings.TrimPrefix(e.Error.Status+": "+e.Error.Message, ": "))
	}
	return string(body)
}

// hintFor picks the trailing guidance for a non-2xx status. A 401 or a
// scope-insufficient 403 is a connection problem (reconnect). A 404, or a 403
// that is a plain permission denial (document exists but is not shared with the
// connected account), is a sharing problem — it must NOT route into reconnect.
func hintFor(status int, body []byte) string {
	e, _ := parseAPIError(body)
	reason := e.reason()
	switch status {
	case http.StatusUnauthorized:
		return scopeHint
	case http.StatusForbidden:
		if reason == "ACCESS_TOKEN_SCOPE_INSUFFICIENT" {
			return scopeHint
		}
		return shareHint
	case http.StatusNotFound:
		return shareHint
	}
	return ""
}

// classifyCredentialError marks only genuine credential rejections (HTTP 401 or
// an explicit UNAUTHENTICATED status) so the engine does not invalidate a
// credential that is still valid. A 403 (scope or permission) never rejects.
func classifyCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	if e, ok := parseAPIError(body); ok && e.Error.Status == "UNAUTHENTICATED" {
		return execution.RejectCredential(err)
	}
	return err
}
