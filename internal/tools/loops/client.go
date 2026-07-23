package loops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, or a malformed key=value pair. It maps to exit code 2
// and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Loops non-2xx response or a transport
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

// call performs one Loops API request with Bearer auth and returns the raw
// response body. A non-2xx surfaces the body's message as an apiError carrying
// the HTTP status; a 401 additionally marks the credential rejected so the
// token gateway learns the key is dead. A transport failure is an apiError with
// status 0.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("loops: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("loops: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
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
		return nil, &apiError{msg: fmt.Sprintf("loops: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("loops: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("loops API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			rejected := execution.RejectCredential(raw)
			return nil, &apiError{msg: rejected.Error(), status: resp.StatusCode, err: rejected}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// callIdempotent is call with an optional Idempotency-Key header (Loops honors
// it on events/send and transactional; 409 on replay). An empty idempotencyKey
// sends no header.
func (s *Service) callIdempotent(ctx context.Context, key, method, path string, query url.Values, payload any, idempotencyKey string) ([]byte, error) {
	if idempotencyKey == "" {
		return s.call(ctx, key, method, path, query, payload)
	}
	// Small duplication of call() so the header stays request-scoped; both
	// paths share apiError shaping.
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("loops: encode request: %v", err), err: err}
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(b))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("loops: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("loops: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("loops: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("loops API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			rejected := execution.RejectCredential(raw)
			return nil, &apiError{msg: rejected.Error(), status: resp.StatusCode, err: rejected}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// apiMessage extracts Loops' error message from an error body: the first-class
// {message} field, then the deprecated {error} field, falling back to the raw
// body.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if e.Message != "" {
			return e.Message
		}
		if e.Error != "" {
			return e.Error
		}
	}
	return string(body)
}

// coerceValue types a raw "key=value" scalar the way Loops' additionalProperties
// (string | number | boolean) expects: "true"/"false" → bool, a valid integer
// or float → number, everything else → string. This keeps custom contact
// properties, event properties, and transactional data variables intention-
// typed without a definition change.
func coerceValue(raw string) any {
	switch raw {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	return raw
}

// parseKeyValues turns repeatable "key=value" flag values into a map with
// typed-coerced values. A pair missing "=" or with an empty key is a usageError.
func parseKeyValues(flag string, pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, &usageError{msg: fmt.Sprintf("loops: --%s %q must be key=value", flag, p)}
		}
		out[k] = coerceValue(v)
	}
	return out, nil
}

// parseMailingLists turns repeatable "id=bool" flag values into the
// MailingListSubscriptions object (id → boolean). The value must be a literal
// true/false; anything else is a usageError.
func parseMailingLists(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(pairs))
	for _, p := range pairs {
		id, v, ok := strings.Cut(p, "=")
		if !ok || id == "" {
			return nil, &usageError{msg: fmt.Sprintf("loops: --mailing-list %q must be id=true|false", p)}
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("loops: --mailing-list %q value must be true or false", p)}
		}
		out[id] = b
	}
	return out, nil
}

// decodeJSONObject validates a raw-JSON flag value expected to be an object and
// returns it for merging into a request body.
func decodeJSONObject(flag, raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("loops: --%s is not a valid JSON object: %v", flag, err)}
	}
	return v, nil
}

// decodeJSONArray validates a raw-JSON flag value expected to be an array and
// returns it for passthrough into a request body.
func decodeJSONArray(flag, raw string) ([]any, error) {
	var v []any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("loops: --%s is not a valid JSON array: %v", flag, err)}
	}
	return v, nil
}
