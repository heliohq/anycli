package acuity

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
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: a malformed --field, a bad enum, or
// any client-side validation failure. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Acuity non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejected classification still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Acuity API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying the HTTP status and Acuity's status_code/message/error.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("acuity: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("acuity: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("acuity: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("acuity: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("acuity API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so the
// engine can invalidate the stored token; other statuses pass through unchanged.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts Acuity's error text (message + error code) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Message != "" || e.Error != "") {
		switch {
		case e.Message != "" && e.Error != "":
			return fmt.Sprintf("%s (%s)", e.Message, e.Error)
		case e.Message != "":
			return e.Message
		default:
			return e.Error
		}
	}
	return strings.TrimSpace(string(body))
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
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

// intakeField is one Acuity intake-form answer: {"id": fieldId, "value": …}.
type intakeField struct {
	ID    int    `json:"id"`
	Value string `json:"value"`
}

// parseFields turns repeatable --field id=value flags into Acuity's fields array.
// A non-integer id or a missing '=' is a usage error (exit 2).
func parseFields(raw []string) ([]intakeField, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	fields := make([]intakeField, 0, len(raw))
	for _, entry := range raw {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, &usageError{msg: fmt.Sprintf("acuity: --field %q must be id=value", entry)}
		}
		id, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("acuity: --field %q has a non-integer id", entry)}
		}
		fields = append(fields, intakeField{ID: id, Value: value})
	}
	return fields, nil
}

// adminEmailQuery builds the shared ?admin=true&noEmail=true query pair from the
// booking/edit flags, omitting each when unset.
func adminEmailQuery(admin, noEmail bool) url.Values {
	q := url.Values{}
	if admin {
		q.Set("admin", "true")
	}
	if noEmail {
		q.Set("noEmail", "true")
	}
	return q
}

// setIfChanged copies an int flag into a body map only when the user set it, so
// unset optional fields never overwrite provider defaults.
func setIntIfChanged(cmd *cobra.Command, body map[string]any, flag, key string, value int) {
	if cmd.Flags().Changed(flag) {
		body[key] = value
	}
}

// setStringIfSet copies a non-empty string into a body map.
func setStringIfSet(body map[string]any, key, value string) {
	if value != "" {
		body[key] = value
	}
}
