package lusha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Lusha non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejected sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Lusha API request with the api_key header and returns the
// raw response body. A 401/403 marks the credential rejected; any other non-2xx
// is an apiError carrying Lusha's message + HTTP status.
func (s *Service) call(ctx context.Context, key, method, path string, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("lusha: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lusha: build request: %v", err), err: err}
	}
	req.Header.Set(apiKeyHeader, key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lusha: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lusha: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("lusha API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks 401/403 as a credential rejection so the host
// (heliox) can prompt a reconnect, per the auth-class error contract.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts Lusha's error message from a response body, falling back
// to the raw body. Lusha error bodies carry a top-level "message" (and
// sometimes "error"); either is surfaced.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != "":
			return e.Error
		}
	}
	return string(body)
}

// emit writes a value as JSON to stdout (+ newline).
func (s *Service) emit(value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("lusha: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// lushaListResponse is the shared shape of Lusha's search-and-enrich, enrich,
// and prospecting responses: a request id, a results array, an optional
// billing block, and (prospecting only) a pagination block.
type lushaListResponse struct {
	RequestID  string           `json:"requestId"`
	Results    json.RawMessage  `json:"results"`
	Billing    *lushaBilling    `json:"billing"`
	Pagination *lushaPagination `json:"pagination"`
}

type lushaBilling struct {
	CreditsCharged  int `json:"creditsCharged"`
	ResultsReturned int `json:"resultsReturned"`
}

type lushaPagination struct {
	Page  int `json:"page"`
	Size  int `json:"size"`
	Total int `json:"total"`
}

// emitRevealEnvelope flattens a search-and-enrich / enrich response into the
// stable envelope: {"data": [<records>], "meta": {credits_charged,
// results_returned, request_id}}. data is always the results array (the
// underlying endpoints are batch-capable, so results is always a list).
func (s *Service) emitRevealEnvelope(body []byte) error {
	var resp lushaListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return &apiError{msg: fmt.Sprintf("lusha: decode response: %v", err), err: err}
	}
	meta := map[string]any{}
	if resp.RequestID != "" {
		meta["request_id"] = resp.RequestID
	}
	if resp.Billing != nil {
		meta["credits_charged"] = resp.Billing.CreditsCharged
		meta["results_returned"] = resp.Billing.ResultsReturned
	}
	return s.emit(map[string]any{"data": rawOrEmptyArray(resp.Results), "meta": meta})
}

// emitSearchEnvelope flattens a prospecting response into the stable envelope:
// {"data": [<preview records>], "meta": {page, size, total, has_more,
// credits_charged, request_id}}. The preview records carry the Lusha id +
// request_id the assistant needs to feed the matching reveal verb.
func (s *Service) emitSearchEnvelope(body []byte) error {
	var resp lushaListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return &apiError{msg: fmt.Sprintf("lusha: decode response: %v", err), err: err}
	}
	meta := map[string]any{}
	if resp.RequestID != "" {
		meta["request_id"] = resp.RequestID
	}
	if resp.Pagination != nil {
		meta["page"] = resp.Pagination.Page
		meta["size"] = resp.Pagination.Size
		meta["total"] = resp.Pagination.Total
		meta["has_more"] = (resp.Pagination.Page+1)*resp.Pagination.Size < resp.Pagination.Total
	}
	if resp.Billing != nil {
		meta["credits_charged"] = resp.Billing.CreditsCharged
	}
	return s.emit(map[string]any{"data": rawOrEmptyArray(resp.Results), "meta": meta})
}

// rawOrEmptyArray returns the raw results as a decoded value, or an empty slice
// when the field is absent, so the envelope's data is always a JSON array.
func rawOrEmptyArray(raw json.RawMessage) any {
	if len(raw) == 0 {
		return []any{}
	}
	return raw
}
