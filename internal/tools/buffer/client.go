package buffer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Buffer non-2xx response, a top-level
// GraphQL `errors` array, a mutation-union error arm, or a transport failure.
// It maps to exit code 1 and kind "api". status is the HTTP status (0 for
// transport failures and GraphQL-level errors on a 200). It wraps the
// underlying cause so errors.As for *credentialRejectedError still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// gqlError is one entry of a GraphQL top-level `errors` array.
type gqlError struct {
	Message string `json:"message"`
}

// gqlResponse is the standard GraphQL envelope. Data is decoded lazily by the
// caller; Errors carries request-level (non-mutation) failures.
type gqlResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []gqlError                 `json:"errors"`
}

// gql performs one GraphQL POST with Bearer auth and returns the decoded top
// level `data` object. A 401 marks the credential rejected; any other non-2xx
// or a non-empty top-level `errors` array is an apiError (fail-fast — never a
// silent empty result).
func (s *Service) gql(ctx context.Context, token, query string, variables map[string]any) (map[string]json.RawMessage, error) {
	payload := map[string]any{"query": query}
	if len(variables) > 0 {
		payload["variables"] = variables
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("buffer: encode request: %v", err), err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL(), bytes.NewReader(body))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("buffer: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("buffer: request failed: %v", err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("buffer: read response: %v", err), err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &apiError{
			msg:    fmt.Sprintf("buffer API error (HTTP %d): %s", resp.StatusCode, httpErrorMessage(raw)),
			status: resp.StatusCode,
		}
		if resp.StatusCode == http.StatusUnauthorized {
			apiErr.err = execution.RejectCredential(fmt.Errorf("%s", apiErr.msg))
			return nil, apiErr
		}
		return nil, apiErr
	}

	var decoded gqlResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("buffer: decode response: %v", err), err: err}
	}
	if len(decoded.Errors) > 0 {
		return nil, &apiError{msg: "buffer API error: " + joinGQLErrors(decoded.Errors)}
	}
	if decoded.Data == nil {
		return nil, &apiError{msg: "buffer API error: response carried no data"}
	}
	return decoded.Data, nil
}

// mutationSuccess reads one mutation field off the GraphQL `data` object and
// enforces the payload union: the field must carry __typename == successType,
// otherwise it is treated as a failure and its `message` (documented on the
// MutationError arm, and on every concrete error type) is surfaced. This holds
// fail-fast even for an error type outside the MutationError interface — any
// non-success typename is an error. Returns the success payload's raw JSON.
func mutationSuccess(data map[string]json.RawMessage, field, successType string) (map[string]any, error) {
	raw, ok := data[field]
	if !ok {
		return nil, &apiError{msg: fmt.Sprintf("buffer: response missing %q payload", field)}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("buffer: decode %q payload: %v", field, err), err: err}
	}
	typename, _ := payload["__typename"].(string)
	if typename != successType {
		msg, _ := payload["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("mutation returned %q", typename)
		}
		return nil, &apiError{msg: "buffer API error: " + msg}
	}
	return payload, nil
}

// decodeField unmarshals one field of the GraphQL `data` object into v.
func decodeField(data map[string]json.RawMessage, field string, v any) error {
	raw, ok := data[field]
	if !ok {
		return &apiError{msg: fmt.Sprintf("buffer: response missing %q", field)}
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return &apiError{msg: fmt.Sprintf("buffer: decode %q: %v", field, err), err: err}
	}
	return nil
}

// emitValue marshals a provider-neutral value and writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("buffer: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(append(body, '\n')); err != nil {
		return err
	}
	return nil
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a GraphQL input. A parse failure is a usage error.
func decodeJSONFlag(name, rawValue string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(rawValue), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
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

// httpErrorMessage extracts a human message from a non-2xx body: a GraphQL
// `errors` array if present, else the raw body (trimmed).
func httpErrorMessage(body []byte) string {
	var decoded gqlResponse
	if err := json.Unmarshal(body, &decoded); err == nil && len(decoded.Errors) > 0 {
		return joinGQLErrors(decoded.Errors)
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// joinGQLErrors joins GraphQL error messages into one string.
func joinGQLErrors(errs []gqlError) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		if e.Message != "" {
			parts = append(parts, e.Message)
		}
	}
	if len(parts) == 0 {
		return "unspecified GraphQL error"
	}
	return strings.Join(parts, "; ")
}
