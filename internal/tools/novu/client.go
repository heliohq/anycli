package novu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// client performs Novu API requests against a resolved region host with the
// literal "Authorization: ApiKey <secret>" scheme.
type client struct {
	host   string
	secret string
	hc     *http.Client
}

// usageError is a parameter / usage error (bad flag combo, missing required
// flag, invalid JSON). It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Novu non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport failures). It wraps its cause so errors.As for the
// credential-rejected classification still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Novu request and returns the raw response body. path must
// include the version prefix (e.g. "/v1/events/trigger", "/v2/subscribers").
// A 401 marks the credential rejected; any other non-2xx is an apiError
// carrying Novu's message and HTTP status.
func (c *client) call(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("novu: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := c.host + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("novu: build request: %v", err), err: err}
	}
	// The literal "ApiKey " prefix is required — Novu rejects "Bearer".
	req.Header.Set("Authorization", "ApiKey "+c.secret)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("novu: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("novu: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("novu API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			classified := execution.RejectCredential(raw)
			return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline). Novu
// envelopes vary per endpoint (a top-level {"data":…} object, a
// {"data":[],page,…} pagination envelope, or a bare array); passing the body
// through unchanged preserves every field — including a trigger's
// status/error/activityFeedLink — without a fragile per-endpoint unwrap.
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Novu's error message from an error body, falling back to
// the raw body. Novu errors carry {message, error, statusCode}; message may be a
// string or an array of validation strings.
func apiMessage(body []byte) string {
	var e struct {
		Message json.RawMessage `json:"message"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if msg := flattenMessage(e.Message); msg != "" {
			return msg
		}
		if e.Error != "" {
			return e.Error
		}
	}
	return string(body)
}

// flattenMessage renders Novu's message field, which is either a JSON string or
// a JSON array of strings (class-validator output).
func flattenMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		out := ""
		for i, m := range arr {
			if i > 0 {
				out += "; "
			}
			out += m
		}
		return out
	}
	return string(raw)
}
