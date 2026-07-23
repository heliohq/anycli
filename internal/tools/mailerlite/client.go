package mailerlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: a missing required flag, a bad enum
// value, or invalid JSON. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a MailerLite non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

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

// call performs one MailerLite Connect API request with Bearer auth and returns
// the raw response body. A 401 marks the credential rejected; any other non-2xx
// becomes an *apiError carrying MailerLite's message/errors. A 204/empty body
// returns an empty slice.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("mailerlite: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mailerlite: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mailerlite: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mailerlite: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{
			msg:    fmt.Sprintf("mailerlite API error (HTTP %d): %s", resp.StatusCode, apiMessage(body)),
			status: resp.StatusCode,
		}
		if resp.StatusCode == http.StatusUnauthorized {
			apiErr.err = execution.RejectCredential(errors.New(apiErr.msg))
			return nil, apiErr
		}
		return nil, apiErr
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline). An
// empty body (204 No Content from delete/forget/cancel/schedule) becomes a
// small success receipt so downstream consumers always get valid JSON.
func (s *Service) emit(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte(`{"success":true}`)
	}
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts MailerLite's error message (and any field-level errors)
// from an error body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Message string              `json:"message"`
		Errors  map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if len(e.Errors) == 0 {
			return e.Message
		}
		var parts []string
		for field, msgs := range e.Errors {
			parts = append(parts, field+": "+strings.Join(msgs, ", "))
		}
		return e.Message + " (" + strings.Join(parts, "; ") + ")"
	}
	return string(body)
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. An empty value is a usage error where a
// JSON body is required; callers that treat empty as "unset" check Changed.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// splitList splits a comma-separated flag value into a trimmed, non-empty slice.
func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// buildBody assembles a request-body map from a set of key/value pairs,
// including only entries whose flag the caller changed. It returns nil when no
// field was set, so callers can send a nil payload (no body) for a no-op.
func buildBody(pairs []bodyField) map[string]any {
	body := map[string]any{}
	for _, f := range pairs {
		if f.set {
			body[f.key] = f.value
		}
	}
	if len(body) == 0 {
		return nil
	}
	return body
}

// bodyField is one candidate entry for buildBody: included only when set.
type bodyField struct {
	key   string
	value any
	set   bool
}

// setLimitCursor applies the shared cursor-paged flags (subscribers/segment
// members/group members/form members) onto a query value set, honoring only
// flags the caller changed.
func setLimitCursor(cmd *cobra.Command, q url.Values, limit int, cursor string) {
	if cmd.Flags().Changed("limit") {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
}

// setLimitPage applies the shared page-paged flags (campaigns/forms/etc.) onto
// a query value set, honoring only flags the caller changed.
func setLimitPage(cmd *cobra.Command, q url.Values, limit, page int) {
	if cmd.Flags().Changed("limit") {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cmd.Flags().Changed("page") {
		q.Set("page", strconv.Itoa(page))
	}
}
