package googleads

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

// usageError is a parameter / usage error: a missing required flag, a bad enum
// value, or an unresolvable argument. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Google Ads non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Google Ads API request with both required headers plus the
// optional login-customer-id, and returns the raw response body. A 401
// marks the credential rejected; any other non-2xx is an apiError carrying the
// HTTP status and Google's nested error message. A transport failure is an
// apiError with status 0.
func (s *Service) call(ctx context.Context, c creds, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("google-ads: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("google-ads: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("developer-token", c.developerToken)
	if c.loginCustomerID != "" {
		req.Header.Set("login-customer-id", c.loginCustomerID)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("google-ads: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("google-ads: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("google-ads API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks 401 (UNAUTHENTICATED) as an explicit credential
// rejection so the engine can invalidate the resolved token. 403 (a scope /
// developer-token access problem) is deliberately NOT a rejection: the bearer
// may be valid but under-privileged, and invalidating it would send the user
// through a pointless reconnect.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// baseURL returns the configured or default API base, trailing slash trimmed.
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

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (the flattened searchStream result)
// and writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("google-ads: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// searchResults is the flattened searchStream shape. searchStream returns a
// JSON array of chunks (a documented quirk unlike every other endpoint's single
// object); flattenStream merges the chunks' rows into one Results array so the
// streamed-array shape never leaks to the caller and query/report emit the same
// object shape whether streamed or paged.
type searchResults struct {
	Results   []json.RawMessage `json:"results"`
	FieldMask string            `json:"fieldMask,omitempty"`
}

// flattenStream parses the searchStream JSON array of chunks and merges every
// chunk's results into a single object. The fieldMask is identical across
// chunks, so the first non-empty one is kept.
func flattenStream(body []byte) (searchResults, error) {
	var chunks []struct {
		Results   []json.RawMessage `json:"results"`
		FieldMask string            `json:"fieldMask"`
	}
	if err := json.Unmarshal(body, &chunks); err != nil {
		return searchResults{}, &apiError{msg: fmt.Sprintf("google-ads: decode searchStream response: %v", err), err: err}
	}
	out := searchResults{Results: []json.RawMessage{}}
	for _, chunk := range chunks {
		out.Results = append(out.Results, chunk.Results...)
		if out.FieldMask == "" && chunk.FieldMask != "" {
			out.FieldMask = chunk.FieldMask
		}
	}
	return out, nil
}

// apiMessage extracts the actionable message from a Google Ads error body. The
// top-level error carries code/message/status; the details[] carry a
// GoogleAdsFailure whose errors[] hold the specific errorCode + message that is
// the real cause (AuthenticationError, QueryError, QuotaError, …). Both are
// surfaced; falls back to the raw body when the shape is unrecognized.
func apiMessage(body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Details []struct {
				Errors []struct {
					ErrorCode map[string]any `json:"errorCode"`
					Message   string         `json:"message"`
				} `json:"errors"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return string(body)
	}
	e := envelope.Error
	if e.Message == "" && e.Status == "" && len(e.Details) == 0 {
		return string(body)
	}
	parts := make([]string, 0, 4)
	if e.Status != "" {
		parts = append(parts, e.Status)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	for _, d := range e.Details {
		for _, ie := range d.Errors {
			detail := ie.Message
			if code := firstErrorCode(ie.ErrorCode); code != "" {
				if detail != "" {
					detail = code + ": " + detail
				} else {
					detail = code
				}
			}
			if detail != "" {
				parts = append(parts, detail)
			}
		}
	}
	if len(parts) == 0 {
		return string(body)
	}
	return strings.Join(parts, "; ")
}

// firstErrorCode renders the single-entry errorCode object (e.g.
// {"queryError":"UNRECOGNIZED_FIELD"}) as "queryError=UNRECOGNIZED_FIELD".
func firstErrorCode(code map[string]any) string {
	for k, v := range code {
		return fmt.Sprintf("%s=%v", k, v)
	}
	return ""
}
