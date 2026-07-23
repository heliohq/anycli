package segment

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
)

// usageError is a parameter / usage error: a missing required flag, a bad value,
// or an unresolvable path. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Segment non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejection marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// paginationQuery builds the dot-notation pagination params the Segment Public
// API accepts (pagination.count / pagination.cursor), per the canonical
// OpenAPI reference (docs.segmentapis.com/tag/Pagination). count is an integer
// 1–1000 (Segment defaults to 200 when omitted); the cursor is the base64
// next/current value from a prior response.
//
// This is the SINGLE place the pagination encoding is defined. Segment's own
// materials disagree (the observability recipe uses bracket notation,
// pagination[count]); the OpenAPI reference — the authoritative spec — uses dot
// notation, so that is what ships. L2 against the live API is the final
// arbiter: if the live API rejects dot notation, only this function changes.
func paginationQuery(count int, cursor string) url.Values {
	q := url.Values{}
	if count > 0 {
		q.Set("pagination.count", strconv.Itoa(count))
	}
	if cursor != "" {
		q.Set("pagination.cursor", cursor)
	}
	return q
}

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// get performs one GET against the Segment Public API with Bearer auth and the
// given query, returning the raw response body. Shared by every read command.
func (s *Service) get(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	return s.do(ctx, token, http.MethodGet, path, query, nil)
}

// do performs one Segment API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is a
// plain apiError carrying Segment's error message and the HTTP status.
func (s *Service) do(ctx context.Context, token, method, path string, query url.Values, payload []byte) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("segment: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("segment: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("segment: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("segment API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := raw
		if resp.StatusCode == http.StatusUnauthorized || isUnauthorizedBody(body) {
			classified = classifyCredentialError(http.StatusUnauthorized, raw)
		}
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
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

// apiMessage extracts a human-readable message from a Segment error body. The
// Public API returns {"errors":[{"type","message","field"}]}; fall back to a
// top-level {"error":…}/{"message":…} shape, then the raw body.
func apiMessage(body []byte) string {
	var env struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Field   string `json:"field"`
		} `json:"errors"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		if len(env.Errors) > 0 {
			e := env.Errors[0]
			switch {
			case e.Message != "" && e.Field != "":
				return e.Message + " (field: " + e.Field + ")"
			case e.Message != "":
				return e.Message
			case e.Type != "":
				return e.Type
			}
		}
		if env.Message != "" {
			return env.Message
		}
		if env.Error != "" {
			return env.Error
		}
	}
	return strings.TrimSpace(string(body))
}
