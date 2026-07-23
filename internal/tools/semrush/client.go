package semrush

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a missing required argument, a bad
// enum value, or an unknown subcommand. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Semrush "ERROR NN :: MESSAGE" body, a
// non-2xx response without an ERROR body, or a transport failure. It maps to
// exit code 1 and kind "api". code is the Semrush ERROR number (0 when the
// failure is transport/HTTP-only); status is the HTTP status (0 for transport
// failures). It wraps the underlying cause so errors.As for a rejected
// credential still resolves through it.
type apiError struct {
	msg    string
	code   int
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// getRaw performs one GET against a Semrush host with the API key in the query
// and returns the raw response body. Semrush report failures arrive as an
// "ERROR NN :: MESSAGE" body — often with HTTP 200 — so the body is returned to
// the caller for ERROR sniffing regardless of a 2xx status; a non-2xx response
// whose body is NOT an ERROR line (an infrastructure failure) becomes an
// apiError carrying the status.
// The caller supplies the fully-formed base URL including its path (Semrush is
// path-sensitive: reports GET "/", backlinks GET "/analytics/v1/", both with a
// trailing slash); getRaw only appends the query.
func (s *Service) getRaw(ctx context.Context, base string, query url.Values, key string) ([]byte, error) {
	query.Set("key", key)
	requestURL := base + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("semrush: build request: %v", err), err: err}
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("semrush: GET: %v", err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("semrush: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if _, _, ok := parseSemrushError(string(body)); !ok {
			return nil, &apiError{
				msg:    fmt.Sprintf("semrush API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body))),
				status: resp.StatusCode,
			}
		}
	}
	return body, nil
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) reportsBaseURL() string {
	if s.ReportsBaseURL != "" {
		return s.ReportsBaseURL
	}
	return DefaultReportsBaseURL
}

func (s *Service) unitsBaseURL() string {
	if s.UnitsBaseURL != "" {
		return s.UnitsBaseURL
	}
	return DefaultUnitsBaseURL
}

// parseSemrushError recognizes Semrush's "ERROR NN :: MESSAGE" plain-text error
// dialect. It returns the numeric code, the message, and whether the body was
// an error line. The separator is "::" but real responses vary the surrounding
// spaces, so parsing is tolerant.
func parseSemrushError(body string) (code int, message string, ok bool) {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "ERROR") {
		return 0, "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "ERROR"))
	// rest is "NN :: MESSAGE" (spacing around :: varies). Split the leading
	// number off the front; everything after the "::" is the message.
	sep := strings.Index(rest, "::")
	numberPart := rest
	if sep >= 0 {
		numberPart = rest[:sep]
		message = strings.TrimSpace(rest[sep+2:])
	}
	n, err := strconv.Atoi(strings.TrimSpace(numberPart))
	if err != nil {
		// "ERROR" prefix but no parseable code — still an error body; surface
		// it with code 0 so callers treat it as a generic API failure.
		return 0, strings.TrimSpace(rest), true
	}
	return n, message, true
}

// nothingFoundCode is Semrush's "no data for this query" signal. It is a valid
// empty answer for an agent, not a failure — the report layer emits empty rows
// with a note and exits 0.
const nothingFoundCode = 50

// classifyReportError maps a parsed ERROR code to the runtime error. Wrong or
// malformed keys (120/121/122) and an API-disabled subscription (130) are
// credential rejections that feed the stale-credential loop; every other code
// (limits 131/132/134, db access 133, and any unknown code) is a plain API
// failure.
func classifyReportError(code int, message string) error {
	msg := fmt.Sprintf("semrush API error (ERROR %d): %s", code, message)
	if message == "" {
		msg = fmt.Sprintf("semrush API error (ERROR %d)", code)
	}
	err := &apiError{msg: msg, code: code}
	switch code {
	case 120, 121, 122, 130:
		return &apiError{msg: msg, code: code, err: execution.RejectCredential(err)}
	default:
		return err
	}
}
