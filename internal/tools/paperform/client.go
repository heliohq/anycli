package paperform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: a missing required flag or an
// unresolvable argument. It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Paperform non-2xx response or a
// transport failure. It maps to exit code 1. status is the HTTP status (0 for
// transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classification still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// get performs one GET against the Paperform v1 API with Bearer auth and
// returns the raw response body. A 401 marks the credential rejected; a 429
// surfaces its Retry-After header; any other non-2xx carries Paperform's
// "message". Transport failures carry status 0.
func (s *Service) get(ctx context.Context, key, path string, query url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("paperform: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("paperform: GET %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("paperform: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, s.classifyError(resp, body)
	}
	return body, nil
}

// classifyError turns a non-2xx response into an apiError, rejecting the
// credential on 401 and appending the Retry-After hint on 429.
func (s *Service) classifyError(resp *http.Response, body []byte) error {
	msg := fmt.Sprintf("paperform API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			msg += fmt.Sprintf(" (rate limited; retry after %s seconds)", ra)
		} else {
			msg += " (rate limited)"
		}
	}
	base := fmt.Errorf("%s", msg)
	if resp.StatusCode == http.StatusUnauthorized {
		return &apiError{msg: msg, status: resp.StatusCode, err: execution.RejectCredential(base)}
	}
	return &apiError{msg: msg, status: resp.StatusCode, err: base}
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Paperform's error "message" from an error body, falling
// back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		return e.Message
	}
	return string(body)
}

// runGet is the shared command body for every read command: build the request,
// call the API, and passthrough the JSON body. All Paperform commands are GETs.
func (s *Service) runGet(cmd *cobra.Command, key, path string, query url.Values) error {
	body, err := s.get(cmd.Context(), key, path, query)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// setIf sets q[name]=value only when value is non-empty, so unset flags never
// override the provider's own defaults.
func setIf(q url.Values, name, value string) {
	if value != "" {
		q.Set(name, value)
	}
}

// intToString renders an int as a base-10 query value.
func intToString(n int) string {
	return strconv.Itoa(n)
}
