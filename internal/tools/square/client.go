package square

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

// call performs one Square Connect API request with Bearer auth and the fixed
// Square-Version header, returning the raw response body. A 401 marks the
// credential rejected; any other non-2xx becomes an *apiError carrying Square's
// errors[].detail. Transport failures become an *apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("square: encode request: %v", err)}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("square: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Square-Version", squareVersion)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("square: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("square: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{
			msg:    fmt.Sprintf("square API error (HTTP %d): %s", resp.StatusCode, apiMessage(body)),
			status: resp.StatusCode,
		}
		if resp.StatusCode == http.StatusUnauthorized {
			apiErr.err = execution.RejectCredential(fmt.Errorf("%s", apiErr.msg))
		}
		return nil, apiErr
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

// apiMessage extracts Square's errors[].detail (falling back to category/code,
// then the raw body) from an error response body.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			Category string `json:"category"`
			Code     string `json:"code"`
			Detail   string `json:"detail"`
			Field    string `json:"field"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && len(e.Errors) > 0 {
		parts := make([]string, 0, len(e.Errors))
		for _, se := range e.Errors {
			switch {
			case se.Detail != "":
				parts = append(parts, se.Detail)
			case se.Code != "":
				parts = append(parts, se.Code)
			case se.Category != "":
				parts = append(parts, se.Category)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	return string(body)
}

// baseURL returns the configured host root without a trailing slash.
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

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. A parse failure is a usage error.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("square: --%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// setNonEmpty sets a query key only when value is non-empty.
func setNonEmpty(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// intToString renders an int as a base-10 query value.
func intToString(n int) string { return strconv.Itoa(n) }
