package zoominfo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// DefaultBaseURL is the ZoomInfo Enterprise API base. Both /authenticate and
// the data endpoints live under this host.
const DefaultBaseURL = "https://api.zoominfo.com"

// usageError is a parameter/usage error (bad flags, missing required flag,
// invalid JSON body, misconfigured credential). It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime/API error: a ZoomInfo non-2xx response, transport
// failure, or signing failure. It maps to exit code 1. status is the HTTP
// status (0 for transport/local failures) and it wraps the cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// classifyCredentialError marks 401/403 as an explicit credential rejection so
// the runtime invalidates the stored PKI credential and prompts a reconnect;
// other statuses stay ordinary failures (the credential may still be valid).
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return execution.RejectCredential(err)
	}
	return err
}

// call performs one authenticated ZoomInfo data request with the resolved
// access JWT. A non-2xx surfaces the response body as an apiError carrying the
// HTTP status; 401/403 additionally classify as credential rejection.
func (s *Service) call(ctx context.Context, accessJWT, method, path string, payload []byte) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if len(payload) > 0 {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoominfo: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+accessJWT)
	req.Header.Set("Accept", "application/json")
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoominfo: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoominfo: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("zoominfo API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}
