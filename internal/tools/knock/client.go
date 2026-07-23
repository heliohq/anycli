package knock

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
// required flag, invalid JSON, or a missing required id. It maps to exit code 2
// and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Knock non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Knock API request with Bearer auth and returns the raw
// response body (possibly empty for a 204). A 401 marks the credential
// rejected; any other non-2xx is an apiError carrying Knock's status/message.
// extraHeaders adds request headers such as Idempotency-Key.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any, extraHeaders map[string]string) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("knock: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("knock: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range extraHeaders {
		req.Header.Set(name, value)
	}

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("knock: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("knock: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("knock API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			classified := execution.RejectCredential(raw)
			return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// callEmit runs one request and writes the result to stdout: the provider JSON
// verbatim, or a compact receipt for an empty 2xx body (Knock returns 204 with
// no body for cancel and some status-mark endpoints).
func (s *Service) callEmit(ctx context.Context, key, method, path string, query url.Values, payload any, extraHeaders map[string]string) error {
	body, err := s.call(ctx, key, method, path, query, payload, extraHeaders)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// emit writes the provider's JSON response to stdout verbatim (+ newline). An
// empty body (204 No Content) becomes a compact {"ok":true} receipt so an agent
// always gets a parseable JSON document.
func (s *Service) emit(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte(`{"ok":true}`)
	}
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Knock's error message from an error body, falling back to
// the raw body. Knock errors carry {message, code, status, type}.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Message != "" || e.Code != "") {
		switch {
		case e.Message != "" && e.Code != "":
			return e.Code + ": " + e.Message
		case e.Message != "":
			return e.Message
		default:
			return e.Code
		}
	}
	return string(body)
}

// decodeJSONFlag validates a raw-JSON flag value into the target shape and
// returns the decoded value for passthrough into a request body. A malformed
// value is a usageError (exit 2), not an API error.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// requireID returns a usageError when a required path id flag is empty.
func requireID(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return &usageError{msg: fmt.Sprintf("--%s is required", name)}
	}
	return nil
}

// addPaging sets Knock's shared list paging params when present.
func addPaging(q url.Values, pageSize int, after, before string) {
	if pageSize > 0 {
		q.Set("page_size", fmt.Sprintf("%d", pageSize))
	}
	if after != "" {
		q.Set("after", after)
	}
	if before != "" {
		q.Set("before", before)
	}
}
