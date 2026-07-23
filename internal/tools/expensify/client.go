package expensify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// apiError is a provider/runtime failure → exit 1. It carries the provider's
// message and, for credential rejection, an execution.RejectCredential cause so
// the engine can invalidate the stored pair.
type apiError struct {
	message string
	cause   error
}

func (e *apiError) Error() string { return e.message }
func (e *apiError) Unwrap() error { return e.cause }

// call injects the credential pair into the job document, POSTs it as the
// requestJobDescription form field, and returns the raw response body. Expensify
// answers HTTP 200 with a JSON body carrying responseCode (401/403 →
// credential rejected); some jobs return a plain body (e.g. a filename), which
// has no responseCode and is returned as success.
func (s *Service) call(ctx context.Context, creds credentials, job map[string]any) ([]byte, error) {
	job["credentials"] = creds
	payload, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("expensify: encode job: %w", err)
	}

	form := url.Values{}
	form.Set("requestJobDescription", string(payload))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("expensify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("expensify: POST: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("expensify: read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &apiError{message: "expensify API error: rate limited (HTTP 429) — retry after a short pause"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &apiError{message: fmt.Sprintf("expensify API error (HTTP %d): %s", resp.StatusCode, providerMessage(body))}
	}

	// HTTP 200: the logical status lives in responseCode.
	if code, msg, ok := responseCodeOf(body); ok && code >= 400 {
		full := fmt.Sprintf("expensify API error (responseCode %d): %s", code, msg)
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			return nil, &apiError{message: full, cause: execution.RejectCredential(fmt.Errorf("%s", full))}
		}
		return nil, &apiError{message: full}
	}
	return body, nil
}

// responseCodeOf reports Expensify's logical status. A body that is not a JSON
// object, or a JSON object without responseCode (e.g. a generated filename or a
// bare success payload), returns ok=false so the caller treats HTTP 200 as
// success and emits the body verbatim.
func responseCodeOf(body []byte) (int, string, bool) {
	var r struct {
		ResponseCode    *float64 `json:"responseCode"`
		ResponseMessage string   `json:"responseMessage"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.ResponseCode == nil {
		return 0, "", false
	}
	msg := strings.TrimSpace(r.ResponseMessage)
	if msg == "" {
		msg = "no message"
	}
	return int(*r.ResponseCode), msg, true
}

// providerMessage extracts responseMessage from an error body, falling back to
// the raw (trimmed) body.
func providerMessage(body []byte) string {
	if _, msg, ok := responseCodeOf(body); ok {
		return msg
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "no message"
	}
	return trimmed
}

// emit writes the provider's response body to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
