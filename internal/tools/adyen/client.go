package adyen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination or a
// missing required scope. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Adyen non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures); errorCode is Adyen's error code when the
// body carries one. It wraps the underlying cause so errors.As for
// *credentialRejectedError still resolves through it (401 → RejectCredential).
type apiError struct {
	msg       string
	status    int
	errorCode string
	err       error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Management API request with raw X-API-Key auth and returns
// the response body. Per Adyen's error docs a 401 is an authentication failure
// (missing/incorrect key) → RejectCredential; every other non-2xx — including a
// role/permission 403 (errorCode 010), which authenticated fine but lacks the
// endpoint role — is an ordinary API error the agent resolves in the Customer
// Area, never a credential rejection.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adyen: build request: %v", err), err: err}
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adyen: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adyen: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code, message := adyenError(body)
		apiErr := &apiError{
			msg:       fmt.Sprintf("adyen API error (HTTP %d): %s", resp.StatusCode, message),
			status:    resp.StatusCode,
			errorCode: code,
		}
		if resp.StatusCode == http.StatusUnauthorized {
			apiErr.err = execution.RejectCredential(fmt.Errorf("%s", apiErr.msg))
			return nil, apiErr
		}
		return nil, apiErr
	}
	return body, nil
}

// adyenError extracts Adyen's standard error errorCode + message from a non-2xx
// body, returning ("", raw-body) when the body is not the standard shape. Adyen
// error bodies carry status/errorCode/message/errorType/pspReference.
func adyenError(body []byte) (code, message string) {
	var e struct {
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.ErrorCode != "" || e.Message != "") {
		switch {
		case e.ErrorCode != "" && e.Message != "":
			return e.ErrorCode, "[" + e.ErrorCode + "] " + e.Message
		case e.Message != "":
			return "", e.Message
		default:
			return e.ErrorCode, "[" + e.ErrorCode + "]"
		}
	}
	return "", string(body)
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
