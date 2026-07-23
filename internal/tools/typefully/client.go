package typefully

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
	"github.com/spf13/cobra"
)

// apiError is a Typefully non-2xx (or transport) failure. kind distinguishes
// the runtime sub-classes so the --json envelope and credential-rejection
// classification stay in one place: "api" (generic non-2xx), "permission" (a
// valid key lacking the required social-set access level), "rate_limit" (429).
type apiError struct {
	status  int
	message string
	kind    string
}

func (e *apiError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("typefully API error (HTTP %d): %s", e.status, e.message)
	}
	return "typefully: " + e.message
}

// usageError is a param/flag error (illegal combo, missing required flag, bad
// JSON). It is NOT an apiError, so Execute maps it to exit 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// call performs one Typefully v2 API request with Bearer auth and returns the
// raw response body. Non-2xx responses are classified into an *apiError; 401
// and auth-related 403 additionally wrap it as a credential rejection.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("typefully: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("typefully: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{message: fmt.Sprintf("%s %s: %v", method, path, err), kind: "api"}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("typefully: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyError(resp.StatusCode, resp.Header, body)
	}
	return body, nil
}

// classifyError maps a non-2xx status + body into the right runtime error.
//   - 401                         -> credential rejected
//   - 403 with an auth-shaped msg -> credential rejected
//   - 403 otherwise (permission)  -> permission runtime error (NOT rejected)
//   - 429                         -> rate-limit runtime error with reset hint
//   - anything else               -> generic api runtime error
func classifyError(status int, header http.Header, body []byte) error {
	msg := apiMessage(body)
	switch {
	case status == http.StatusUnauthorized:
		return execution.RejectCredential(&apiError{status: status, message: msg, kind: "api"})
	case status == http.StatusForbidden && isAuthMessage(msg):
		return execution.RejectCredential(&apiError{status: status, message: msg, kind: "api"})
	case status == http.StatusForbidden:
		// Valid key, insufficient per-social-set access level (READ/WRITE/
		// PUBLISH/ADMIN). Surface as a distinct non-credential runtime error so
		// the connection is not wrongly flagged for reconnect.
		return &apiError{status: status, message: msg + " (this looks like a permissions problem: the key lacks the required access level on this social set, not an invalid key)", kind: "permission"}
	case status == http.StatusTooManyRequests:
		return &apiError{status: status, message: msg + rateLimitHint(header), kind: "rate_limit"}
	default:
		return &apiError{status: status, message: msg, kind: "api"}
	}
}

// isAuthMessage reports whether a 403 body reads as an authentication failure
// (invalid/missing key) rather than a permission/access-level denial.
func isAuthMessage(msg string) bool {
	l := strings.ToLower(msg)
	for _, needle := range []string{"authentication", "invalid api key", "invalid token", "invalid key", "unauthorized", "not authenticated"} {
		if strings.Contains(l, needle) {
			return true
		}
	}
	return false
}

// rateLimitHint surfaces the reset window from the standard X-RateLimit-* headers
// so the agent can back off rather than auto-retry.
func rateLimitHint(header http.Header) string {
	reset := header.Get("X-RateLimit-Reset")
	if reset == "" {
		reset = header.Get("Retry-After")
	}
	if reset == "" {
		return " (rate limited; retry later, do not auto-retry)"
	}
	return " (rate limited; reset in/at " + reset + ", do not auto-retry)"
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (media receipts) and writes it to
// stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("typefully: encode output: %w", err)
	}
	return s.emit(body)
}

// apiMessage extracts Typefully's error message from an error body, falling back
// to the raw body. Typefully v2 errors carry {"error": {...}} or a top-level
// {"detail"|"message"|"code"} shape depending on the failure class.
func apiMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	var envelope struct {
		Detail  string `json:"detail"`
		Message string `json:"message"`
		Code    string `json:"code"`
		Error   struct {
			Detail  string `json:"detail"`
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		for _, candidate := range []string{
			envelope.Detail, envelope.Message, envelope.Code,
			envelope.Error.Detail, envelope.Error.Message, envelope.Error.Code,
		} {
			if candidate != "" {
				return candidate
			}
		}
	}
	return trimmed
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
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

// addPaging maps --limit / --offset onto a query value set when set (>0 limit,
// >=0 offset). Callers pass the provider's own defaults through by leaving them
// unset (the service never re-caps; Typefully applies its documented caps).
func addPaging(q url.Values, limit, offset int) {
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
}

// registerPaging wires the shared --limit/--offset flags (0 = omit, use the
// provider default).
func registerPaging(cmd *cobra.Command, limit, offset *int) {
	cmd.Flags().IntVar(limit, "limit", 0, "max results (omitted = provider default)")
	cmd.Flags().IntVar(offset, "offset", 0, "result offset for pagination")
}
