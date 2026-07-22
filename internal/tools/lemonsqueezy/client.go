package lemonsqueezy

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
)

// usageError is a parameter / usage error: a malformed flag value, missing
// required flag, or invalid JSON body. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Lemon Squeezy non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// sideEffect builds the design-318 annotation map for a runnable leaf.
func sideEffect(mutates bool) map[string]string {
	if mutates {
		return map[string]string{"anycli.side_effect": "true"}
	}
	return map[string]string{"anycli.side_effect": "false"}
}

// baseURL returns the configured base with any trailing slash trimmed.
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

// get issues a GET and emits the response body verbatim on stdout. query may be
// nil.
func (s *Service) get(ctx context.Context, token, path string, query url.Values) error {
	body, err := s.call(ctx, token, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// send issues a mutating request (POST/PATCH/DELETE) and emits the response
// body. A DELETE that returns 204 with an empty body emits nothing.
func (s *Service) send(ctx context.Context, token, method, path string, query url.Values, payload json.RawMessage) error {
	body, err := s.call(ctx, token, method, path, query, payload)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	return s.emit(body)
}

// emit writes the provider's JSON response to stdout verbatim (plus a
// newline).
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Lemon Squeezy request. It sets the Bearer token and both
// required vnd.api+json headers on every call. A non-2xx surfaces the body's
// JSON:API error (status/title/detail) as an apiError carrying the HTTP
// status; a transport failure is an apiError with status 0. A 401 is marked as
// a credential rejection so the engine can invalidate the stored key.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload json.RawMessage) ([]byte, error) {
	u := s.baseURL() + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lemon-squeezy: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", mediaType)
	req.Header.Set("Content-Type", mediaType)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lemon-squeezy: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lemon-squeezy: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("lemon-squeezy API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := raw
		if resp.StatusCode == http.StatusUnauthorized {
			classified = execution.RejectCredential(raw)
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// apiMessage extracts Lemon Squeezy's JSON:API error (title + detail from the
// first errors[] entry), falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && len(e.Errors) > 0 {
		first := e.Errors[0]
		switch {
		case first.Title != "" && first.Detail != "":
			return fmt.Sprintf("%s: %s", first.Title, first.Detail)
		case first.Detail != "":
			return first.Detail
		case first.Title != "":
			return first.Title
		}
	}
	return strings.TrimSpace(string(body))
}

// listQuery builds the JSON:API list query from flat flags: --page →
// page[number], --page-size → page[size], --filter k=v (repeatable) →
// filter[k]=v, --include a,b → include=a,b. A --filter entry without "=" is a
// usage error so the AI never silently drops a filter.
func listQuery(page, pageSize int, filters []string, include string) (url.Values, error) {
	q := url.Values{}
	if page > 0 {
		q.Set("page[number]", fmt.Sprintf("%d", page))
	}
	if pageSize > 0 {
		q.Set("page[size]", fmt.Sprintf("%d", pageSize))
	}
	for _, f := range filters {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, &usageError{msg: fmt.Sprintf("--filter %q must be key=value", f)}
		}
		q.Set("filter["+k+"]", v)
	}
	if include != "" {
		q.Set("include", include)
	}
	return q, nil
}

// includeQuery builds the query for a single-resource GET's --include.
func includeQuery(include string) url.Values {
	q := url.Values{}
	if include != "" {
		q.Set("include", include)
	}
	return q
}

// paramQuery builds a flat key=value query from repeatable --param flags (used
// by generate-invoice, whose fields are query parameters).
func paramQuery(params []string) (url.Values, error) {
	q := url.Values{}
	for _, p := range params {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, &usageError{msg: fmt.Sprintf("--param %q must be key=value", p)}
		}
		q.Set(k, v)
	}
	return q, nil
}

// parseData validates a JSON:API request body. Empty is allowed only for
// optional bodies (the caller decides). Invalid JSON is a fail-fast usage
// error.
func parseData(val string) (json.RawMessage, error) {
	if strings.TrimSpace(val) == "" {
		return nil, nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(val), &raw); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--data is not valid JSON: %v", err)}
	}
	return raw, nil
}
