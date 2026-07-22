package mercury

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a missing required flag or an
// illegal flag value. It maps to exit code 2 and error kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Mercury non-2xx response, a transport
// failure, or a decode failure. It maps to exit code 1 and kind "api". status
// is the HTTP status (0 for transport/network failures). It wraps the
// underlying cause so errors.As for a credential-rejection sentinel still
// resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// baseURL returns the configured base or the production default.
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

// call performs one Mercury API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected (so the token gateway
// refreshes); any other non-2xx surfaces Mercury's message as an apiError
// carrying the HTTP status; a transport failure is an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mercury: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mercury: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mercury: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("mercury API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			rejected := execution.RejectCredential(raw)
			return nil, &apiError{msg: rejected.Error(), status: resp.StatusCode, err: rejected}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// emitObject wraps a single fetched resource body in the {"data": <object>}
// envelope and writes it to stdout. The provider object's own fields are
// preserved verbatim.
func (s *Service) emitObject(body []byte) error {
	out := map[string]json.RawMessage{"data": json.RawMessage(body)}
	return s.emitJSON(out)
}

// emitList extracts the listKey array from a Mercury list envelope and re-emits
// it as {"data": [...]}, carrying through any of metaKeys that are present
// (e.g. "total", "page") so an agent can paginate. A missing/empty list becomes
// an empty array rather than null.
func (s *Service) emitList(body []byte, listKey string, metaKeys ...string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return &apiError{msg: fmt.Sprintf("mercury: decode list response: %v", err), err: err}
	}
	data := raw[listKey]
	if len(data) == 0 {
		data = json.RawMessage("[]")
	}
	out := map[string]json.RawMessage{"data": data}
	for _, k := range metaKeys {
		if v, ok := raw[k]; ok {
			out[k] = v
		}
	}
	return s.emitJSON(out)
}

// emitJSON marshals an envelope and writes it to stdout with a trailing newline.
func (s *Service) emitJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("mercury: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(b); err != nil {
		return err
	}
	_, err = io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Mercury's error message from an error body, trying the
// common shapes (top-level message/error, nested errors.message), and falling
// back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Errors  struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Errors.Message != "":
			return e.Errors.Message
		case e.Error != "":
			return e.Error
		}
	}
	return string(body)
}
