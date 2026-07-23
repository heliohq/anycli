package onesignal

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

// intToString renders an int as a base-10 query value.
func intToString(n int) string { return strconv.Itoa(n) }

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or a violated targeting rule. It
// maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a OneSignal non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status
// (0 for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one OneSignal API request with "Authorization: Key <key>" auth
// and returns the raw response body. A 401 marks the credential rejected; any
// other non-2xx surfaces OneSignal's error message as an apiError carrying the
// HTTP status, and a transport failure as an apiError with status 0.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("onesignal: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("onesignal: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Key "+key)
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
		return nil, &apiError{msg: fmt.Sprintf("onesignal: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("onesignal: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("onesignal API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			// The App API Key was rejected (revoked/wrong): invalidate it.
			rejected := execution.RejectCredential(raw)
			return nil, &apiError{msg: rejected.Error(), status: resp.StatusCode, err: rejected}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
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

// appQuery returns a query value set pre-seeded with the auto-injected app_id.
func appQuery(appID string) url.Values {
	q := url.Values{}
	q.Set("app_id", appID)
	return q
}

// appPath builds an app-scoped path (/apps/{app_id}<suffix>) with the app_id
// escaped into the path segment.
func appPath(appID, suffix string) string {
	return "/apps/" + url.PathEscape(appID) + suffix
}

// apiMessage extracts OneSignal's error text from a response body. OneSignal
// returns errors as {"errors": ["msg", ...]} or {"errors": {"field": [...]}}
// depending on endpoint; fall back to the raw body when neither parses.
func apiMessage(body []byte) string {
	// Array form: {"errors": ["Message is a required field", ...]}
	var arr struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(body, &arr); err == nil && len(arr.Errors) > 0 {
		return strings.Join(arr.Errors, "; ")
	}
	// Object/map form: {"errors": {"app_id": ["is invalid"]}} or a bare
	// {"error": "..."} string.
	var obj struct {
		Errors map[string]any `json:"errors"`
		Error  string         `json:"error"`
	}
	if err := json.Unmarshal(body, &obj); err == nil {
		if obj.Error != "" {
			return obj.Error
		}
		if len(obj.Errors) > 0 {
			parts := make([]string, 0, len(obj.Errors))
			for field, v := range obj.Errors {
				parts = append(parts, fmt.Sprintf("%s: %v", field, v))
			}
			return strings.Join(parts, "; ")
		}
	}
	return string(body)
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}
