package razorpay

import (
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

// call performs one Razorpay API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is a
// typed apiError carrying Razorpay's error code/description.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("razorpay: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("razorpay: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("razorpay: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("razorpay API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: execution.RejectCredential(raw)}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
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

// apiMessage extracts Razorpay's error code/description from an error body,
// falling back to the raw body. Razorpay wraps 4xx errors as
// {"error":{"code","description","source","step","reason","metadata"}}.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error.Code != "" || e.Error.Description != "") {
		switch {
		case e.Error.Code != "" && e.Error.Description != "":
			return e.Error.Code + ": " + e.Error.Description
		case e.Error.Description != "":
			return e.Error.Description
		default:
			return e.Error.Code
		}
	}
	return string(body)
}

// listParams holds the pagination + time-window flags every Razorpay list
// endpoint accepts: count (max 100) + skip offset, plus optional from/to Unix
// timestamp bounds. Each is sent only when the caller set it, so the provider
// applies its own defaults otherwise.
type listParams struct {
	count int
	skip  int
	from  string
	to    string
}

// register wires --count / --skip / --from / --to onto a list command.
func (p *listParams) register(cmd *cobra.Command) {
	cmd.Flags().IntVar(&p.count, "count", 0, "number of records to fetch (max 100)")
	cmd.Flags().IntVar(&p.skip, "skip", 0, "number of records to skip (pagination offset)")
	cmd.Flags().StringVar(&p.from, "from", "", "Unix timestamp lower bound (inclusive)")
	cmd.Flags().StringVar(&p.to, "to", "", "Unix timestamp upper bound (inclusive)")
}

// query builds the URL query from the flags the caller actually set.
func (p listParams) query(cmd *cobra.Command) url.Values {
	q := url.Values{}
	if cmd.Flags().Changed("count") {
		q.Set("count", strconv.Itoa(p.count))
	}
	if cmd.Flags().Changed("skip") {
		q.Set("skip", strconv.Itoa(p.skip))
	}
	if p.from != "" {
		q.Set("from", p.from)
	}
	if p.to != "" {
		q.Set("to", p.to)
	}
	return q
}
