package mailjet

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

	"github.com/heliohq/anycli/internal/tools/execution"
)

// apiError is a runtime/API failure (Mailjet non-2xx or transport error). It
// carries the HTTP status for the --json error envelope and wraps the cause so
// a 401/403 credential rejection propagates through errors.As/Is.
type apiError struct {
	status  int
	message string
	cause   error
}

func (e *apiError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("mailjet API error (HTTP %d): %s", e.status, e.message)
	}
	return "mailjet: " + e.message
}

func (e *apiError) Unwrap() error { return e.cause }

// usageError is a local input problem (bad flag combo/enum) → exit 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// call performs one Mailjet API request with Basic auth and returns the raw
// response body. A 401/403 marks the credential rejected; any other non-2xx is
// an apiError carrying Mailjet's error message.
func (s *Service) call(ctx context.Context, basic, baseURL, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{message: fmt.Sprintf("encode request: %v", err)}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{message: fmt.Sprintf("build request: %v", err)}
	}
	req.Header.Set("Authorization", basicAuthHeader(basic))
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{message: fmt.Sprintf("%s %s: %v", method, path, err), cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{message: fmt.Sprintf("read response: %v", err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{status: resp.StatusCode, message: apiMessage(body)}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			apiErr.cause = execution.RejectCredential(errors.New(apiErr.Error()))
		}
		return nil, apiErr
	}
	return body, nil
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// restEnvelope is the shared v3 REST response shape. Data is kept as raw JSON so
// the unwrap helpers surface list/get payloads without re-encoding drift.
type restEnvelope struct {
	Count int               `json:"Count"`
	Total int               `json:"Total"`
	Data  []json.RawMessage `json:"Data"`
}

// emit writes raw JSON to stdout verbatim (+ newline) — used for the Send API
// v3.1 response, which is not a REST envelope.
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value and writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{message: fmt.Sprintf("encode output: %v", err)}
	}
	return s.emit(body)
}

// emitList unwraps a v3 REST envelope into {"data":[…],"count":N,"total":T},
// dropping the provider's {"Count","Data","Total"} wrapper while keeping each
// record's native field names (matches the notion/bitly verbatim-record
// precedent). Pagination counts are preserved because agents page on them.
func (s *Service) emitList(body []byte) error {
	env, err := decodeEnvelope(body)
	if err != nil {
		return err
	}
	data := env.Data
	if data == nil {
		data = []json.RawMessage{}
	}
	return s.emitValue(map[string]any{
		"data":  data,
		"count": env.Count,
		"total": env.Total,
	})
}

// emitOne unwraps a v3 REST envelope to its single record (Data[0]) for get-by-id
// calls, emitting null when the resource set is empty.
func (s *Service) emitOne(body []byte) error {
	env, err := decodeEnvelope(body)
	if err != nil {
		return err
	}
	if len(env.Data) == 0 {
		return s.emit([]byte("null"))
	}
	return s.emit(env.Data[0])
}

func decodeEnvelope(body []byte) (restEnvelope, error) {
	var env restEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return restEnvelope{}, &apiError{message: fmt.Sprintf("decode response: %v", err)}
	}
	return env, nil
}

// apiMessage extracts Mailjet's error text. v3 REST errors carry
// {"ErrorInfo","ErrorMessage","StatusCode"}; Send API v3.1 errors carry
// {"ErrorMessage"} or a Messages[].Errors[] list. Falls back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		ErrorMessage string `json:"ErrorMessage"`
		ErrorInfo    string `json:"ErrorInfo"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.ErrorMessage != "" && e.ErrorInfo != "":
			return e.ErrorMessage + " (" + e.ErrorInfo + ")"
		case e.ErrorMessage != "":
			return e.ErrorMessage
		}
	}
	if len(body) == 0 {
		return "(empty response body)"
	}
	return string(body)
}

// itoa / itoa64 render ints as base-10 query values.
func itoa(n int) string { return strconv.Itoa(n) }

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// decodeJSONFlag validates a raw-JSON flag value for passthrough into a body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}
