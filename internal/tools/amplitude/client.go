package amplitude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or malformed credentials. It
// maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Amplitude non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// invocation is the per-command resolved request target: the Basic auth header,
// the region-selected base URL, and whether the caller asserted the region
// explicitly (which governs 401 credential classification, see auth_error.go).
type invocation struct {
	authHeader     string
	baseURL        string
	regionExplicit bool
}

// resolve reads the persistent region / base-url flags and the pre-built auth
// header into a per-command invocation. Service.BaseURL (test seam) wins over
// the region-selected host, but regionExplicit still tracks whether --region
// was passed so classification tests stay meaningful.
func (s *Service) resolve(cmd *cobra.Command, authHeader string) (*invocation, error) {
	region, _ := cmd.Flags().GetString("region")
	baseFlag, _ := cmd.Flags().GetString("base-url")
	regionExplicit := cmd.Flags().Changed("region")

	base := ""
	switch {
	case s.BaseURL != "":
		base = s.BaseURL
	case baseFlag != "":
		base = baseFlag
	case region == "us":
		base = usBaseURL
	case region == "eu":
		base = euBaseURL
	default:
		return nil, &usageError{msg: fmt.Sprintf("--region must be us or eu, got %q", region)}
	}
	return &invocation{
		authHeader:     authHeader,
		baseURL:        strings.TrimRight(base, "/"),
		regionExplicit: regionExplicit,
	}, nil
}

// newRequest builds one authenticated Amplitude request. The Basic header is
// set from the resolved invocation; query values are URL-encoded (Amplitude
// requires URL-encoded JSON for the e=/s= parameters).
func (inv *invocation) newRequest(ctx context.Context, method, path string, query url.Values) (*http.Request, error) {
	requestURL := inv.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("amplitude: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", inv.authHeader)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// client returns the configured HTTP client or the default.
func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// call performs one Amplitude request and returns the raw response body. A
// non-2xx surfaces Amplitude's message as an apiError carrying the HTTP status;
// a 401 is classified (region-aware) into a credential rejection or an EU-retry
// hint. A transport failure is an apiError with status 0.
func (s *Service) call(ctx context.Context, inv *invocation, method, path string, query url.Values) ([]byte, error) {
	req, err := inv.newRequest(ctx, method, path, query)
	if err != nil {
		return nil, err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("amplitude: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("amplitude: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(inv, resp.StatusCode, body)
	}
	return body, nil
}

// newAPIError builds the apiError for a non-2xx response, running the
// region-aware 401 classifier so the wrapped cause carries either a credential
// rejection or the EU-retry hint.
func newAPIError(inv *invocation, status int, body []byte) *apiError {
	raw := fmt.Errorf("amplitude API error (HTTP %d): %s", status, apiMessage(body))
	classified := classifyCredentialError(inv.regionExplicit, status, raw)
	return &apiError{msg: classified.Error(), status: status, err: classified}
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (CSV envelope / export receipt) and
// writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("amplitude: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// apiMessage extracts Amplitude's error text from an error body, falling back
// to the raw body. Amplitude uses several shapes across its APIs (error,
// message, and a nested details string), so all are probed.
func apiMessage(body []byte) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Details string `json:"details"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Error != "":
			return e.Error
		case e.Message != "":
			return e.Message
		case e.Details != "":
			return e.Details
		}
	}
	return strings.TrimSpace(string(body))
}

// parseJSONFlag validates a raw-JSON flag value. An empty value yields nil;
// invalid JSON is a fail-fast usage error. The value is passed through to the
// query param as the original compact string so Amplitude's large segment
// grammar is never re-modeled here.
func parseJSONFlag(name, val string) (string, error) {
	if strings.TrimSpace(val) == "" {
		return "", nil
	}
	var probe any
	if err := json.Unmarshal([]byte(val), &probe); err != nil {
		return "", &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return val, nil
}
