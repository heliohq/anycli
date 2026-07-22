package calcom

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

// Per-endpoint cal-api-version dates. Cal.com v2 pins the API version PER
// ENDPOINT FAMILY, not with a single global constant: sending the wrong (or an
// absent) date silently downgrades an endpoint's response semantics — and some
// v2 endpoints 404 outright without it. Each command sends its own version;
// a single fixed value would be a bug. Confirmed against each endpoint's
// official reference (cal.com/docs/api-reference/v2).
const (
	verEventTypes = "2024-06-14" // /v2/event-types, /v2/me
	verSlots      = "2024-09-04" // /v2/slots
	verBookings   = "2024-08-13" // /v2/bookings (list/get/create/cancel/reschedule)
	verSchedules  = "2024-06-11" // /v2/schedules
	verMe         = "2024-06-14" // /v2/me
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Cal.com non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status
// (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classification still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// envelope is Cal.com v2's uniform response wrapper: {"status":"success"|
// "error","data":…} on success, {"status":"error","error":…} on failure.
type envelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  json.RawMessage `json:"error"`
}

// getJSON performs a GET on path with the endpoint's cal-api-version, unwraps
// the v2 envelope, and returns the raw `data` payload.
func (s *Service) getJSON(ctx context.Context, token, path, version string, query url.Values) ([]byte, error) {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return s.call(ctx, token, http.MethodGet, path, version, nil)
}

// postJSON performs a POST on path with the endpoint's cal-api-version and a
// JSON body, unwraps the v2 envelope, and returns the raw `data` payload.
func (s *Service) postJSON(ctx context.Context, token, path, version string, payload any) ([]byte, error) {
	return s.call(ctx, token, http.MethodPost, path, version, payload)
}

// call performs one Cal.com v2 request: Bearer auth + the per-route
// cal-api-version header on every call, then unwraps the {status,data}
// envelope. A non-2xx surfaces Cal.com's error body verbatim as an apiError
// carrying the HTTP status (401 is classified as a credential rejection); a
// transport failure surfaces as an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path, version string, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("calcom: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("calcom: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("cal-api-version", version)
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
		return nil, &apiError{msg: fmt.Sprintf("calcom: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("calcom: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("calcom API error (HTTP %d): %s", resp.StatusCode, errorMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return unwrap(body)
}

// unwrap extracts the v2 envelope's `data`. If the body is a bare (non-enveloped)
// JSON value it is returned as-is, so the service stays robust to endpoints that
// do not wrap. A `status:error` envelope on a 2xx (rare) surfaces as an apiError.
func unwrap(body []byte) ([]byte, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil || env.Status == "" {
		// Not an enveloped response — return verbatim.
		return body, nil
	}
	if env.Status == "error" {
		return nil, &apiError{msg: fmt.Sprintf("calcom API error: %s", string(env.Error))}
	}
	if len(env.Data) > 0 {
		return env.Data, nil
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so
// the engine can invalidate the token; other statuses pass through unchanged.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// errorMessage extracts Cal.com's error message from a {status:error,error:…}
// body, falling back to the raw body when it is not the expected shape.
func errorMessage(body []byte) string {
	var env struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil && len(env.Error) > 0 {
		var detail struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(env.Error, &detail) == nil && (detail.Code != "" || detail.Message != "") {
			if detail.Code != "" && detail.Message != "" {
				return detail.Code + ": " + detail.Message
			}
			return detail.Code + detail.Message
		}
		return string(env.Error)
	}
	return string(body)
}

// emitJSON writes a JSON payload to stdout verbatim (newline-terminated).
func (s *Service) emitJSON(data []byte) error {
	_, err := s.stdout().Write(append(data, '\n'))
	return err
}
