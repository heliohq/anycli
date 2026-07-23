package pinterest

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, or bad enum value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Pinterest non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classifier still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Pinterest v5 API request with Bearer auth and returns the
// raw response body. A 401 marks the credential rejected with an explicit
// reconnect hint; a 429 surfaces a rate-limit backoff hint; any other non-2xx
// carries Pinterest's code/message.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("pinterest: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pinterest: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pinterest: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pinterest: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("pinterest API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), statusHint(resp.StatusCode))
		classified := raw
		if resp.StatusCode == http.StatusUnauthorized {
			classified = execution.RejectCredential(raw)
		}
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// emit writes the provider's JSON response to stdout verbatim (+ newline). List
// responses carry the `bookmark` cursor inline, so surfacing the raw body is
// exactly how the assistant reads the next-page token.
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// statusHint returns an actionable clause for the failures an agent most often
// hits, so the assistant reconnects / backs off / fixes input rather than
// retrying blindly.
func statusHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return " (token expired or revoked — reconnect the Pinterest connection)"
	case http.StatusForbidden:
		return " (insufficient scope, or the image is too small/large/broken for a pin)"
	case http.StatusNotFound:
		return " (check the board_id / pin_id — the resource was not found)"
	case http.StatusTooManyRequests:
		return " (rate limited — back off and retry later)"
	}
	return ""
}

// apiMessage extracts Pinterest's error message (code + message) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if e.Code != 0 {
			return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
		}
		return e.Message
	}
	return string(body)
}
