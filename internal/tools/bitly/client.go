package bitly

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

// call performs one Bitly API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is a
// plain error carrying Bitly's message/description.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("bitly: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("bitly: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitly: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bitly: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("bitly API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}

// resolveGroup returns explicit when non-empty, otherwise reads the
// authenticated user's default_group_guid from GET /user. The lookup runs at
// most once per command invocation (callers resolve lazily).
func (s *Service) resolveGroup(ctx context.Context, token, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	body, err := s.call(ctx, token, http.MethodGet, "/user", nil, nil)
	if err != nil {
		return "", err
	}
	var u struct {
		DefaultGroupGUID string `json:"default_group_guid"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return "", fmt.Errorf("bitly: decode user: %w", err)
	}
	if u.DefaultGroupGUID == "" {
		return "", fmt.Errorf("bitly: no default_group_guid on user; pass --group explicitly")
	}
	return u.DefaultGroupGUID, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (receipts / image envelopes) and
// writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("bitly: encode output: %w", err)
	}
	return s.emit(body)
}

// apiMessage extracts Bitly's error message/description from an error body,
// falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Message     string `json:"message"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Message != "" || e.Description != "") {
		switch {
		case e.Message != "" && e.Description != "":
			return e.Message + ": " + e.Description
		case e.Message != "":
			return e.Message
		default:
			return e.Description
		}
	}
	return string(body)
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("bitly: --%s is not valid JSON: %w", name, err)
	}
	return v, nil
}

// analyticsParams holds the shared unit-window flags Bitly's metric endpoints
// accept. unit is always sent; units is sent as an integer (-1 = all); the
// reference timestamp is omitted when empty.
type analyticsParams struct {
	unit          string
	units         int
	unitReference string
}

// registerAnalyticsFlags wires --unit / --units / --unit-reference onto a
// metric command, with the design defaults (day, -1, empty).
func registerAnalyticsFlags(cmd *cobra.Command, a *analyticsParams) {
	cmd.Flags().StringVar(&a.unit, "unit", "day", "time unit: minute|hour|day|week|month")
	cmd.Flags().IntVar(&a.units, "units", -1, "number of units (-1 = all available)")
	cmd.Flags().StringVar(&a.unitReference, "unit-reference", "", "ISO-8601 reference timestamp (optional)")
}

// apply writes the analytics params into a query value set.
func (a analyticsParams) apply(q url.Values) {
	q.Set("unit", a.unit)
	q.Set("units", fmt.Sprintf("%d", a.units))
	if a.unitReference != "" {
		q.Set("unit_reference", a.unitReference)
	}
}

// registerSizeFlag wires the optional breakdown --size flag (default 50).
func registerSizeFlag(cmd *cobra.Command, size *int) {
	cmd.Flags().IntVar(size, "size", 50, "max entries in the breakdown")
}

// intToString renders an int as a base-10 query value.
func intToString(n int) string {
	return strconv.Itoa(n)
}
