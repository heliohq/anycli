package plaid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: an unsupported PLAID_ENV, a
// sandbox-only command run against production, a missing required flag, or a bad
// enum value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Plaid non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". It carries Plaid's own error
// envelope fields (error_type / error_code / request_id) verbatim so an agent
// can self-correct, and wraps the underlying cause so errors.As for the
// credential-rejection marker still resolves through it. status is the HTTP
// status (0 for transport failures).
type apiError struct {
	msg       string
	status    int
	errorType string
	errorCode string
	requestID string
	err       error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// plaidErrorBody is Plaid's standard error envelope, returned on every non-2xx.
type plaidErrorBody struct {
	ErrorType    string `json:"error_type"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	RequestID    string `json:"request_id"`
}

// credentialRejectingCodes are the Plaid error_codes that mean the stored APP
// credential (client_id/secret) is bad — so the token gateway should treat the
// connection as needing re-auth. INVALID_ACCESS_TOKEN and ITEM_LOGIN_REQUIRED
// are deliberately excluded: those concern the per-invocation Item access_token
// (runtime data), not the stored connection credential.
var credentialRejectingCodes = map[string]bool{
	"INVALID_API_KEYS":  true,
	"INVALID_CLIENT_ID": true,
	"INVALID_SECRET":    true,
	"UNAUTHORIZED":      true,
}

// call performs one Plaid API POST. client_id/secret ride the PLAID-CLIENT-ID /
// PLAID-SECRET headers; the payload carries the request-specific fields (e.g. an
// Item access_token). A non-2xx surfaces Plaid's error envelope as an apiError;
// an error_code identifying a bad app credential is additionally marked
// credential-rejected.
func (s *Service) call(ctx context.Context, c creds, path string, payload map[string]any) ([]byte, error) {
	body := map[string]any{}
	for k, v := range payload {
		body[k] = v
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("plaid: encode request: %v", err), err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("plaid: build request: %v", err), err: err}
	}
	req.Header.Set("PLAID-CLIENT-ID", c.clientID)
	req.Header.Set("PLAID-SECRET", c.secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("plaid: POST %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("plaid: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp.StatusCode, respBody)
	}
	return respBody, nil
}

// newAPIError builds an apiError from a Plaid non-2xx response, decoding the
// error envelope and classifying credential rejection by error_code.
func newAPIError(status int, body []byte) *apiError {
	var pe plaidErrorBody
	_ = json.Unmarshal(body, &pe)

	msg := plaidErrorMessage(status, pe, body)
	raw := fmt.Errorf("%s", msg)
	classified := raw
	if credentialRejectingCodes[pe.ErrorCode] {
		classified = execution.RejectCredential(raw)
	}
	return &apiError{
		msg:       msg,
		status:    status,
		errorType: pe.ErrorType,
		errorCode: pe.ErrorCode,
		requestID: pe.RequestID,
		err:       classified,
	}
}

// plaidErrorMessage renders a human-readable one-line message from Plaid's error
// envelope, falling back to the raw body when it cannot be decoded.
func plaidErrorMessage(status int, pe plaidErrorBody, body []byte) string {
	if pe.ErrorCode != "" || pe.ErrorType != "" || pe.ErrorMessage != "" {
		detail := pe.ErrorMessage
		if detail == "" {
			detail = pe.ErrorType
		}
		return fmt.Sprintf("plaid API error (HTTP %d): %s: %s", status, pe.ErrorCode, detail)
	}
	return fmt.Sprintf("plaid API error (HTTP %d): %s", status, string(body))
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
