package moz

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a missing required argument, a bad
// --data JSON value, or an unknown subcommand. It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a JSON-RPC error object, a non-2xx
// response, or a transport failure. It maps to exit code 1. code is the
// JSON-RPC error code (0 when the failure is transport/HTTP-only); status is
// the HTTP status (0 for transport failures). It wraps the underlying cause so
// errors.As for a rejected credential still resolves through it.
type apiError struct {
	msg    string
	code   int
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// jsonRPCRequest is the JSON-RPC 2.0 envelope Moz expects. params always wraps
// the method payload under a `data` key, per the Moz API convention.
type jsonRPCRequest struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      string    `json:"id"`
	Method  string    `json:"method"`
	Params  rpcParams `json:"params"`
}

type rpcParams struct {
	Data any `json:"data"`
}

// jsonRPCResponse is the JSON-RPC 2.0 reply. Exactly one of Result / Error is
// populated for a well-formed response.
type jsonRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *jsonRPCError   `json:"error"`
}

// jsonRPCError is the Moz JSON-RPC error object. Its `status` mirrors the HTTP
// status; data.explanation carries the human-readable cause.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Explanation string `json:"explanation"`
		Issue       string `json:"issue"`
		Key         string `json:"key"`
	} `json:"data"`
}

// call performs one Moz JSON-RPC request: it wraps method + data in the 2.0
// envelope with a fresh request id, POSTs it with the x-moz-token header, and
// returns the raw `result` value for verbatim passthrough. A JSON-RPC error
// object (or a non-2xx transport response) becomes an apiError; HTTP 401/403
// marks the credential rejected.
func (s *Service) call(ctx context.Context, token, method string, data any) (json.RawMessage, error) {
	reqBody, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      s.requestID(),
		Method:  method,
		Params:  rpcParams{Data: data},
	})
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("moz: encode request: %v", err), err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("moz: build request: %v", err), err: err}
	}
	req.Header.Set("x-moz-token", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("moz: POST %s: %v", method, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("moz: read response: %v", err), err: err}
	}

	var parsed jsonRPCResponse
	// A JSON-RPC error is signalled by the `error` member; a non-2xx status
	// without a decodable body is an infrastructure failure. Attempt to decode
	// either way so a structured Moz error surfaces even on a non-2xx status.
	decodeErr := json.Unmarshal(body, &parsed)
	if parsed.Error != nil {
		return nil, classifyRPCError(resp.StatusCode, parsed.Error)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		apiErr := &apiError{
			msg:    fmt.Sprintf("moz API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body))),
			status: resp.StatusCode,
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			apiErr.err = execution.RejectCredential(errors.New(apiErr.msg))
		}
		return nil, apiErr
	}
	if decodeErr != nil {
		return nil, &apiError{msg: fmt.Sprintf("moz: decode response: %v", decodeErr), status: resp.StatusCode, err: decodeErr}
	}
	if parsed.Result == nil {
		// A 2xx with neither result nor error is a contract violation, not data.
		return nil, &apiError{msg: "moz: response carried neither result nor error", status: resp.StatusCode}
	}
	return parsed.Result, nil
}

// classifyRPCError maps a Moz JSON-RPC error object onto an apiError. The
// object's own `status` is preferred over the HTTP status (they mirror each
// other); 401/403 marks the credential rejected so the host invalidates it.
func classifyRPCError(httpStatus int, rpcErr *jsonRPCError) error {
	status := rpcErr.Status
	if status == 0 {
		status = httpStatus
	}
	msg := rpcErr.Message
	if rpcErr.Data.Explanation != "" {
		if msg != "" {
			msg += ": " + rpcErr.Data.Explanation
		} else {
			msg = rpcErr.Data.Explanation
		}
	}
	if msg == "" {
		msg = fmt.Sprintf("JSON-RPC error %d", rpcErr.Code)
	}
	apiErr := &apiError{
		msg:    fmt.Sprintf("moz API error (code %d, HTTP %d): %s", rpcErr.Code, status, msg),
		code:   rpcErr.Code,
		status: status,
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		apiErr.err = execution.RejectCredential(errors.New(apiErr.msg))
	}
	return apiErr
}

// requestID returns a JSON-RPC request id. The Moz API requires an id of at
// least 24 characters and recommends a V4 UUID; a 36-char dashed UUIDv4
// satisfies both. Tests may override via newRequestID.
func (s *Service) requestID() string {
	if s.newRequestID != nil {
		return s.newRequestID()
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unrecoverable here; fall back to a padded,
		// >=24-char literal so we never emit an id the API would reject.
		return "helio-moz-0000000000000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

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

// emit writes the provider's raw JSON result to stdout verbatim (+ newline).
func (s *Service) emit(result json.RawMessage) error {
	if _, err := s.stdout().Write(result); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
