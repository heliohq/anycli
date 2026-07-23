package courier

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
// required flag, or invalid JSON. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Courier non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Courier API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying Courier's message and the HTTP status.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("courier: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("courier: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("courier: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("courier: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("courier API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			rejected := execution.RejectCredential(raw)
			return nil, &apiError{msg: rejected.Error(), status: resp.StatusCode, err: rejected}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline). An
// empty body (204 No Content on subscribe/unsubscribe) writes nothing.
func (s *Service) emit(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Courier's error message from an error body, falling back
// to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if e.Type != "" {
			return e.Type + ": " + e.Message
		}
		return e.Message
	}
	return string(body)
}

// decodeJSONObjectFlag validates a raw-JSON-object flag value and returns the
// decoded map for passthrough into a request body. A decode failure is a
// usageError (exit 2).
func decodeJSONObjectFlag(name, raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("courier: --%s is not a valid JSON object: %v", name, err)}
	}
	return v, nil
}

// setIf adds key=value to q only when value is non-empty.
func setIf(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}
