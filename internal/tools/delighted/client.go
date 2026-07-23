package delighted

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

// call performs one Delighted API request with HTTP Basic auth (API key as the
// username, blank password) and returns the raw response body. A 401 marks the
// credential rejected; any other non-2xx is a plain error carrying Delighted's
// message. path must already include the ".json" suffix.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("delighted: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("delighted: build request: %w", err)
	}
	// Delighted authenticates with the API key as the Basic username and an
	// empty password (Authorization: Basic base64(<key>:)).
	req.SetBasicAuth(key, "")
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("delighted: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("delighted: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("delighted API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Delighted's error message from an error body. Delighted
// returns either {"message": "..."} or {"error": "..."}; fall back to the raw
// body when neither is present.
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

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("delighted: --%s is not valid JSON: %w", name, err)
	}
	return v, nil
}

// intToString renders an int as a base-10 query value.
func intToString(n int) string {
	return strconv.Itoa(n)
}

// setIfNonEmpty writes a query parameter only when the value is non-empty.
func setIfNonEmpty(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// registerPaging wires the shared --per-page / --page list flags onto a
// command and returns pointers to the bound values.
func registerPaging(cmd *cobra.Command) (perPage, page *int) {
	perPage = cmd.Flags().Int("per-page", 0, "results per page (0 = provider default)")
	page = cmd.Flags().Int("page", 0, "page number (0 = provider default)")
	return perPage, page
}

// applyPaging writes the shared paging flags into a query value set.
func applyPaging(q url.Values, perPage, page int) {
	if perPage > 0 {
		q.Set("per_page", intToString(perPage))
	}
	if page > 0 {
		q.Set("page", intToString(page))
	}
}
