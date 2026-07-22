package apollo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, or invalid JSON in a --body flag. It maps to exit
// code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Apollo non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejection sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Apollo API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying Apollo's message and the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("apollo: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("apollo: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("apollo: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("apollo: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("apollo API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), accessHint(resp.StatusCode))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: execution.RejectCredential(raw)}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// accessHint returns an actionable clause for a 403, which Apollo returns when
// an OAuth token calls a master-API-key-only endpoint (people search, sequence
// add/remove, deal list/update).
func accessHint(status int) string {
	if status == http.StatusForbidden {
		return " (this endpoint may require an Apollo master API key; an OAuth token cannot reach it)"
	}
	return ""
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Apollo's error text from an error body, falling back to
// the raw body. Apollo variously returns {"error":"..."}, {"errors":["..."]},
// or {"message":"..."}.
func apiMessage(body []byte) string {
	var e struct {
		Error   string   `json:"error"`
		Errors  []string `json:"errors"`
		Message string   `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Error != "":
			return e.Error
		case len(e.Errors) > 0:
			return e.Errors[0]
		case e.Message != "":
			return e.Message
		}
	}
	return string(body)
}

// bodyFromFlag decodes a raw JSON --body flag into a mutable object map that
// typed flags then augment. An empty flag yields an empty map. A non-object or
// malformed JSON value is a usage error (exit 2). This is the escape hatch that
// lets an agent pass any documented Apollo filter/field the typed flags do not
// name, without this tool having to enumerate Apollo's full schema.
func bodyFromFlag(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("apollo: --body is not a valid JSON object: %v", err)}
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// decodeJSONArray decodes a raw JSON array flag (e.g. --filters-json) for
// passthrough into a request field. A malformed value is a usage error.
func decodeJSONArray(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("apollo: --%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// setStr sets key on body when val is non-empty (typed flags augment --body).
func setStr(body map[string]any, key, val string) {
	if val != "" {
		body[key] = val
	}
}

// setStrSlice sets key on body when the slice is non-empty.
func setStrSlice(body map[string]any, key string, vals []string) {
	if len(vals) > 0 {
		body[key] = vals
	}
}

// registerBodyFlag wires the shared raw-JSON --body passthrough onto a command.
func registerBodyFlag(cmd *cobra.Command, body *string) {
	cmd.Flags().StringVar(body, "body", "", "raw JSON object merged as the request body base (typed flags override)")
}

// registerPageFlags wires the shared --page / --per-page pagination flags.
func registerPageFlags(cmd *cobra.Command, page, perPage *int) {
	cmd.Flags().IntVar(page, "page", 0, "page number (1-based; omitted when 0)")
	cmd.Flags().IntVar(perPage, "per-page", 0, "results per page (omitted when 0)")
}

// applyPageBody sets page / per_page into a POST-search request body when > 0.
func applyPageBody(body map[string]any, page, perPage int) {
	if page > 0 {
		body["page"] = page
	}
	if perPage > 0 {
		body["per_page"] = perPage
	}
}

// applyPageQuery sets page / per_page into a GET query when > 0.
func applyPageQuery(q url.Values, page, perPage int) {
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
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
