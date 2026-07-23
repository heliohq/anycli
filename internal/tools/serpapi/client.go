package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a malformed --param value or other
// client-side input problem. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a SerpApi non-2xx response, a transport
// failure, or a missing credential on an authed call. It maps to exit code 1
// and kind "api". status is the HTTP status (0 for non-HTTP failures). It
// wraps the underlying cause so errors.As for the credential-rejection marker
// still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// get performs one SerpApi GET. When apiKey is non-empty it is injected as
// the `api_key` query parameter — SerpApi's only supported auth placement (no
// header auth exists in the official docs). Callers of authed endpoints must
// pass a non-empty key (see requireKey); the free Locations API passes "".
// A 401 marks the credential rejected; any other non-2xx is an apiError
// carrying SerpApi's top-level "error" string.
func (s *Service) get(ctx context.Context, apiKey, path string, query url.Values) ([]byte, error) {
	if query == nil {
		query = url.Values{}
	}
	if apiKey != "" {
		query.Set("api_key", apiKey)
	}
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("serpapi: build request: %v", err), err: err}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("serpapi: GET %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("serpapi: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		cause := fmt.Errorf("serpapi API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			cause = execution.RejectCredential(cause)
		}
		return nil, &apiError{msg: cause.Error(), status: resp.StatusCode, err: cause}
	}
	return body, nil
}

// requireKey guards authed endpoints: SerpApi rejects keyless calls anyway,
// but failing locally is explicit and never burns a request.
func requireKey(apiKey string) error {
	if apiKey == "" {
		return &apiError{msg: EnvAPIKey + " is not set"}
	}
	return nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts SerpApi's top-level "error" string from an error body,
// falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error != "" {
		return e.Error
	}
	return string(body)
}
