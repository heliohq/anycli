package ramp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: a bad flag combination, missing
// required argument, or invalid input. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Ramp non-2xx response or a transport
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

// emitJSON writes the provider's JSON response to stdout verbatim.
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// get performs one authenticated GET against path (which must start with "/"
// and carry its own /developer/v1 prefix), with optional query parameters.
// Bearer auth on every call; a non-2xx surfaces the body's message as an
// apiError carrying the HTTP status, and a transport failure as an apiError
// with status 0. A 401 (or a Ramp-signalled credential rejection) is
// classified so the Helio token gateway's refresh path fires (design 227).
func (s *Service) get(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	full := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("ramp: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("ramp: GET %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("ramp: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("ramp API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so
// exit-1 carries the signal the token gateway needs to refresh-and-retry.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts Ramp's error message from an error body. Ramp's error
// shapes vary by endpoint: a nested {"error":{"message":…}} envelope, a
// top-level {"message":…}, or the OAuth-style {"error_description"/"error"}.
// Fall back to the raw body when none is present.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message          string `json:"message"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if e.Error.Message != "" {
			return e.Error.Message
		}
		if e.Message != "" {
			return e.Message
		}
		if e.ErrorDescription != "" {
			return e.ErrorDescription
		}
	}
	// OAuth-style bodies carry a bare string `error` field, which does not fit
	// the struct above (where `error` is an object). Try it separately.
	var s struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &s); err == nil && s.Error != "" {
		return s.Error
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// pageFlags holds the cursor-pagination flags shared by every list command.
type pageFlags struct {
	all    bool
	limit  int
	cursor string
}

// registerPaginationFlags attaches --all / --limit / --cursor to a list command.
func registerPaginationFlags(cmd *cobra.Command) *pageFlags {
	pf := &pageFlags{}
	cmd.Flags().BoolVar(&pf.all, "all", false, "fetch every page by following page.next")
	cmd.Flags().IntVar(&pf.limit, "limit", 0, "max items per page (Ramp page_size)")
	cmd.Flags().StringVar(&pf.cursor, "cursor", "", "resume from a prior response's start cursor")
	return pf
}

// listQuery builds the query parameters for a paginated list request from the
// pagination flags plus a starting cursor. Ramp's wire params are page_size
// (limit) and start (cursor).
func listQuery(limit int, cursor string) url.Values {
	q := url.Values{}
	if limit > 0 {
		q.Set("page_size", fmt.Sprintf("%d", limit))
	}
	if cursor != "" {
		q.Set("start", cursor)
	}
	return q
}

// nextStart extracts the `start` cursor from a page.next URL. Ramp returns
// page.next as a full URL (e.g.
// https://api.ramp.com/developer/v1/transactions?start=<uuid>&page_size=…);
// the tool re-issues against the same path with just the start cursor.
func nextStart(nextURL string) string {
	u, err := url.Parse(nextURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("start")
}

// runList runs a paginated GET over Ramp's cursor envelope ({data, page.next}).
// Without --all it returns the first page verbatim (data + page.next intact for
// manual continuation). With --all it follows page.next until it is empty,
// accumulating data into a single envelope.
func (s *Service) runList(ctx context.Context, token, path string, pf *pageFlags, extra url.Values) ([]byte, error) {
	fetch := func(cursor string) ([]byte, error) {
		q := listQuery(pf.limit, cursor)
		for k, vs := range extra {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		return s.get(ctx, token, path, q)
	}
	if !pf.all {
		return fetch(pf.cursor)
	}
	var acc []json.RawMessage
	cursor := pf.cursor
	for {
		body, err := fetch(cursor)
		if err != nil {
			return nil, err
		}
		var env struct {
			Data []json.RawMessage `json:"data"`
			Page struct {
				Next *string `json:"next"`
			} `json:"page"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, &apiError{msg: fmt.Sprintf("ramp: decode list page: %v", err), err: err}
		}
		acc = append(acc, env.Data...)
		if env.Page.Next == nil || *env.Page.Next == "" {
			break
		}
		next := nextStart(*env.Page.Next)
		if next == "" {
			break
		}
		cursor = next
	}
	if acc == nil {
		acc = []json.RawMessage{}
	}
	return json.Marshal(map[string]any{"data": acc, "page": map[string]any{"next": nil}})
}
