package tally

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// httpDoer is the minimal HTTP surface the service needs; *http.Client
// satisfies it and tests can point it at an httptest server's client.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// apiError is a Tally non-2xx response. It carries the HTTP status so the error
// envelope can surface it and Execute can classify runtime failures (exit 1).
type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("tally API error (HTTP %d): %s", e.status, e.message)
}

// usageError is a parameter/usage failure (bad flag combo, invalid JSON body).
// Execute maps it — and every cobra parse error — to exit 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// call performs one Tally API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx becomes
// an apiError carrying Tally's message.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, body []byte) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("tally: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("tally: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tally: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{status: resp.StatusCode, message: apiMessage(respBody)}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return respBody, nil
}

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() httpDoer {
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

// apiMessage extracts Tally's error message from an error body, falling back to
// the raw body. Tally does not declare a stable error schema, so both the
// common {message} shape and a bare string are handled.
func apiMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != "":
			return e.Error
		}
	}
	return trimmed
}

// readBody resolves a request body from --file or --stdin and validates it is
// JSON. Exactly one source must be provided.
func (s *Service) readBody(file string, stdin bool) ([]byte, error) {
	var (
		raw []byte
		err error
	)
	switch {
	case file != "" && stdin:
		return nil, &usageError{msg: "provide only one of --file or --stdin"}
	case file != "":
		raw, err = os.ReadFile(file)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --file: %v", err)}
		}
	case stdin:
		raw, err = io.ReadAll(s.stdin())
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --stdin: %v", err)}
		}
	default:
		return nil, &usageError{msg: "provide a request body with --file <path> or --stdin"}
	}
	if !json.Valid(raw) {
		return nil, &usageError{msg: "request body is not valid JSON"}
	}
	return raw, nil
}

// oneOfFlag validates that value is in allowed, returning a usageError otherwise.
func oneOfFlag(name, value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return &usageError{msg: fmt.Sprintf("--%s must be one of %s", name, strings.Join(allowed, "|"))}
}

// setIf adds a query value only when non-empty.
func setIf(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// bodyFlags registers the shared --file/--stdin body-source flags.
func bodyFlags(cmd *cobra.Command, file *string, stdin *bool) {
	cmd.Flags().StringVar(file, "file", "", "read the JSON request body from a file")
	cmd.Flags().BoolVar(stdin, "stdin", false, "read the JSON request body from stdin")
}
