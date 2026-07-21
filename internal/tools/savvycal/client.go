package savvycal

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

// itoa renders an int as a base-10 query value.
func itoa(n int) string { return strconv.Itoa(n) }

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or a malformed --field pair. It
// maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a SavvyCal non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one SavvyCal API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying SavvyCal's error body and the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("savvycal: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("savvycal: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("savvycal: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("savvycal: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("savvycal API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		cause := error(raw)
		if resp.StatusCode == http.StatusUnauthorized {
			cause = execution.RejectCredential(raw)
		}
		return nil, &apiError{msg: cause.Error(), status: resp.StatusCode, err: cause}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts a human-readable message from a SavvyCal error body,
// preferring the {"errors": {...}} validation shape and the {"error": "..."}
// shape, and falling back to the raw body so nothing is lost.
func apiMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	// Validation errors arrive as {"errors": {...}}; surface the whole object
	// so the agent sees which fields failed. A scalar {"error": "..."} and a
	// {"message": "..."} shape are also honored.
	var envelope struct {
		Errors  json.RawMessage `json:"errors"`
		Error   string          `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		switch {
		case len(envelope.Errors) > 0 && string(envelope.Errors) != "null":
			return string(envelope.Errors)
		case envelope.Error != "":
			return envelope.Error
		case envelope.Message != "":
			return envelope.Message
		}
	}
	return trimmed
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. A malformed value is a usage error.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("savvycal: --%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// parseFields turns repeatable --field id=value pairs into the SavvyCal
// fields[] array shape ([{id, value}]). A pair without "=" is a usage error.
func parseFields(pairs []string) ([]map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(pairs))
	for _, p := range pairs {
		id, value, ok := strings.Cut(p, "=")
		if !ok || id == "" {
			return nil, &usageError{msg: fmt.Sprintf("savvycal: --field %q must be id=value", p)}
		}
		out = append(out, map[string]any{"id": id, "value": value})
	}
	return out, nil
}
