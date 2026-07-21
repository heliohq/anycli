package salesloft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// maxPerPage is Salesloft's per_page ceiling; a larger value is a usage error
// rather than a silently clamped request.
const maxPerPage = 100

// apiPrefix is the API version segment prepended to every request path, so the
// per-endpoint path literals stay version-relative (e.g. "/me" → "/v2/me").
const apiPrefix = "/v2"

// usageError is a parameter / usage error: bad flag combination, missing
// required flag, invalid JSON, or an out-of-range value. It maps to exit code 2
// and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Salesloft non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classifier still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Salesloft API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying Salesloft's error string or field-error map and the HTTP
// status. A transport failure is an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("salesloft: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + apiPrefix + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("salesloft: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("salesloft: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("salesloft: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("salesloft API error (HTTP %d): %s%s",
			resp.StatusCode, apiMessage(body), rateLimitHint(resp))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so the
// engine can invalidate the stored token; other statuses stay ordinary.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// rateLimitHint appends the remaining-minute budget to a 429 message so an agent
// can back off; empty for every other status.
func rateLimitHint(resp *http.Response) string {
	if resp.StatusCode != http.StatusTooManyRequests {
		return ""
	}
	remaining := resp.Header.Get("x-ratelimit-remaining-minute")
	return fmt.Sprintf(" (rate limit: x-ratelimit-remaining-minute=%s)", remaining)
}

// apiMessage extracts Salesloft's error text: the `error` string (403/404) or
// the `errors` field→messages map (422), falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error  string              `json:"error"`
		Errors map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if e.Error != "" {
			return e.Error
		}
		if len(e.Errors) > 0 {
			fields := make([]string, 0, len(e.Errors))
			for field := range e.Errors {
				fields = append(fields, field)
			}
			sort.Strings(fields) // deterministic order across a Go map
			parts := make([]string, 0, len(fields))
			for _, field := range fields {
				parts = append(parts, field+": "+strings.Join(e.Errors[field], ", "))
			}
			return strings.Join(parts, "; ")
		}
	}
	return string(body)
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// listFlags holds the shared list controls every Salesloft list endpoint
// accepts: 1-based paging, incremental updated_at filtering, sorting, and a
// generic repeatable filter passthrough for any documented query filter the
// tool does not surface as a named flag.
type listFlags struct {
	page          int
	perPage       int
	updatedSince  string
	sortBy        string
	sortDirection string
	filters       []string
}

// registerListFlags wires the shared list controls onto cmd.
func registerListFlags(cmd *cobra.Command, lf *listFlags) {
	cmd.Flags().IntVar(&lf.page, "page", 0, "1-based page number")
	cmd.Flags().IntVar(&lf.perPage, "per-page", 0, "results per page (max 100)")
	cmd.Flags().StringVar(&lf.updatedSince, "updated-since", "", "only records updated at/after this ISO-8601 time (updated_at[gte])")
	cmd.Flags().StringVar(&lf.sortBy, "sort-by", "", "field to sort by (e.g. updated_at)")
	cmd.Flags().StringVar(&lf.sortDirection, "sort-direction", "", "sort direction: ASC|DESC")
	cmd.Flags().StringArrayVar(&lf.filters, "filter", nil, "extra query filter key=value (repeatable; value is sent verbatim, e.g. person_stage_id[]=7)")
}

// values renders the shared list flags into a query set, validating per-page
// against the Salesloft ceiling and parsing each --filter into a query param.
func (lf listFlags) values() (url.Values, error) {
	if lf.perPage > maxPerPage {
		return nil, &usageError{msg: fmt.Sprintf("--per-page %d exceeds the Salesloft maximum of %d", lf.perPage, maxPerPage)}
	}
	q := url.Values{}
	if lf.page > 0 {
		q.Set("page", strconv.Itoa(lf.page))
	}
	if lf.perPage > 0 {
		q.Set("per_page", strconv.Itoa(lf.perPage))
	}
	if lf.updatedSince != "" {
		q.Set("updated_at[gte]", lf.updatedSince)
	}
	if lf.sortBy != "" {
		q.Set("sort_by", lf.sortBy)
	}
	if lf.sortDirection != "" {
		q.Set("sort_direction", lf.sortDirection)
	}
	for _, f := range lf.filters {
		key, value, ok := strings.Cut(f, "=")
		if !ok || key == "" {
			return nil, &usageError{msg: fmt.Sprintf("--filter %q must be key=value", f)}
		}
		q.Add(key, value)
	}
	return q, nil
}

// mergeBody overlays a raw --body JSON object onto the named-flag map: named
// fields provide convenient defaults, --body keys override them for full
// request fidelity when the named flags do not cover a field. An empty raw body
// leaves named as-is; a non-object raw body is a usage error.
func mergeBody(named map[string]any, raw string) (map[string]any, error) {
	if raw == "" {
		return named, nil
	}
	var override map[string]any
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--body is not a valid JSON object: %v", err)}
	}
	for k, v := range override {
		named[k] = v
	}
	return named, nil
}
