package missive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// maxRetryAfter caps how long a single 429 retry waits, so a hostile or absurd
// Retry-After cannot hang the agent's tool call.
const maxRetryAfter = 10 * time.Second

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, invalid JSON, or a bad enum. It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Missive non-2xx response or a transport
// failure. It maps to exit code 1. status is the HTTP status (0 for transport
// failures); retryAfter is the surfaced 429 Retry-After (seconds, 0 when
// absent). It wraps the underlying cause so errors.As for the credential-
// rejection marker still resolves through it.
type apiError struct {
	msg        string
	status     int
	retryAfter int
	err        error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Missive API request with Bearer auth and returns the raw
// response body. A 429 is retried once after honoring a bounded Retry-After; a
// 401/403 marks the credential rejected; any other non-2xx is an apiError
// carrying Missive's message and HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var raw []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("missive: encode request: %v", err), err: err}
		}
		raw = b
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	for attempt := 0; attempt < 2; attempt++ {
		var reqBody io.Reader
		if payload != nil {
			reqBody = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("missive: build request: %v", err), err: err}
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := s.client().Do(req)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("missive: %s %s: %v", method, path, err), err: err}
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, &apiError{msg: fmt.Sprintf("missive: read response: %v", readErr), err: readErr}
		}
		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			return body, nil
		}

		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			s.sleepBackoff(retryAfter)
			continue
		}
		return nil, s.classifyError(resp.StatusCode, retryAfter, body)
	}
	// Unreachable: the loop either returns a body, retries once, or returns an
	// error on the second attempt.
	return nil, &apiError{msg: "missive: request retry exhausted"}
}

// classifyError builds the apiError for a non-2xx response, marking 401/403 as
// a credential rejection so the engine surfaces stale-credential feedback.
func (s *Service) classifyError(status, retryAfter int, body []byte) error {
	msg := fmt.Sprintf("missive API error (HTTP %d): %s", status, apiMessage(body))
	if status == http.StatusTooManyRequests && retryAfter > 0 {
		msg = fmt.Sprintf("%s (retry after %ds)", msg, retryAfter)
	}
	apiErr := &apiError{msg: msg, status: status, retryAfter: retryAfter}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		apiErr.err = execution.RejectCredential(&plainError{msg})
		return apiErr
	}
	apiErr.err = &plainError{msg}
	return apiErr
}

// plainError carries an error string without any classification; it is the
// wrapped cause so errors.As for the credential-rejection marker can traverse
// an apiError to reach a RejectCredential wrapper.
type plainError struct{ msg string }

func (e *plainError) Error() string { return e.msg }

// sleepBackoff pauses before a 429 retry, honoring a bounded Retry-After (a
// small floor when the header is absent). The sleeper is a test seam.
func (s *Service) sleepBackoff(retryAfter int) {
	delay := time.Duration(retryAfter) * time.Second
	if delay <= 0 {
		delay = time.Second
	}
	if delay > maxRetryAfter {
		delay = maxRetryAfter
	}
	if s.sleep != nil {
		s.sleep(delay)
		return
	}
	time.Sleep(delay)
}

// parseRetryAfter reads the integer-seconds Retry-After header Missive returns
// on 429. A missing or non-integer value yields 0.
func parseRetryAfter(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitBodyOrOK writes the response verbatim, or {"ok":true} when Missive
// returns a success with an empty body (e.g. a 201 with no content).
func (s *Service) emitBodyOrOK(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return s.emit([]byte(`{"ok":true}`))
	}
	return s.emit(body)
}

// apiMessage extracts Missive's error message from an error body, falling back
// to the raw body. Missive errors are JSON objects; the message may be a
// top-level "message" string or nested under "errors".
func apiMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	var e struct {
		Message string `json:"message"`
		Errors  any    `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if e.Message != "" {
			return e.Message
		}
		if e.Errors != nil {
			if encoded, mErr := json.Marshal(e.Errors); mErr == nil {
				return string(encoded)
			}
		}
	}
	return trimmed
}

// decodeJSONBody reads a request body from a --json inline value or a --file
// path ("-" = stdin), returning the decoded value for passthrough. Exactly one
// source must be provided.
func (s *Service) decodeJSONBody(inline, file string, stdin io.Reader) (any, error) {
	if inline != "" && file != "" {
		return nil, &usageError{msg: "provide either --json or --file, not both"}
	}
	var rawJSON []byte
	switch {
	case inline != "":
		rawJSON = []byte(inline)
	case file == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read body from stdin: %v", err)}
		}
		rawJSON = b
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read body from file: %v", err)}
		}
		rawJSON = b
	default:
		return nil, &usageError{msg: "a request body is required: pass --json '<payload>' or --file <path|->"}
	}
	var v any
	if err := json.Unmarshal(rawJSON, &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("request body is not valid JSON: %v", err)}
	}
	return v, nil
}
