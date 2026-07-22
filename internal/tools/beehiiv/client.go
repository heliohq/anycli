package beehiiv

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
// required flag, bad enum value, invalid JSON, or a malformed id. It maps to
// exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a beehiiv non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classifier still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one beehiiv API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying beehiiv's errors[].message and the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("beehiiv: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("beehiiv: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("beehiiv: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("beehiiv: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("beehiiv API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
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
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts beehiiv's error text from an error body. beehiiv returns
// {"errors":[{"message":…}]} (sometimes {"error":…} or {"message":…}); fall
// back to the raw body when none is present.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		var msgs []string
		for _, item := range e.Errors {
			if item.Message != "" {
				msgs = append(msgs, item.Message)
			}
		}
		if len(msgs) > 0 {
			return strings.Join(msgs, "; ")
		}
		if e.Message != "" {
			return e.Message
		}
		if e.Error != "" {
			return e.Error
		}
	}
	return string(body)
}

// requirePublicationID validates a --publication-id flag value. beehiiv
// publication ids are prefixed `pub_`; a missing or malformed value is a usage
// error (exit 2) with an actionable hint rather than a downstream 404.
func requirePublicationID(id string) (string, error) {
	if id == "" {
		return "", &usageError{msg: "--publication-id is required (run `beehiiv publication list` to find it)"}
	}
	if !strings.HasPrefix(id, "pub_") {
		return "", &usageError{msg: fmt.Sprintf("--publication-id %q is malformed (expected a pub_… id; run `beehiiv publication list`)", id)}
	}
	return id, nil
}

// decodeJSONObject parses a raw-JSON flag value into a string-keyed object for
// use as (or merged into) a request body. A non-object or invalid JSON is a
// usage error.
func decodeJSONObject(flag, raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not a valid JSON object: %v", flag, err)}
	}
	return v, nil
}

// setIfNonEmpty sets a query key only when the value is non-empty, keeping the
// wire query clean of unset filters.
func setIfNonEmpty(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// addExpand appends each expand value as a repeated `expand[]` query param,
// beehiiv's documented array-query convention.
func addExpand(q url.Values, expand []string) {
	for _, e := range expand {
		if e != "" {
			q.Add("expand[]", e)
		}
	}
}
