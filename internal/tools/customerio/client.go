package customerio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or an unknown region. It maps to
// exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Customer.io non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejected classifier still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Customer.io App API request with Bearer auth against the
// region resolved from the command. A 401 marks the credential rejected; any
// other non-2xx is an apiError carrying the HTTP status and provider message. A
// transport failure is an apiError with status 0.
func (s *Service) call(cmd *cobra.Command, key, method, path string, query url.Values, payload any) ([]byte, error) {
	base, err := s.regionBase(cmd)
	if err != nil {
		return nil, err
	}
	var reqBody io.Reader
	if payload != nil {
		b, mErr := json.Marshal(payload)
		if mErr != nil {
			return nil, &apiError{msg: fmt.Sprintf("customer-io: encode request: %v", mErr), err: mErr}
		}
		reqBody = bytes.NewReader(b)
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(cmd.Context(), method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("customer-io: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
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
		return nil, &apiError{msg: fmt.Sprintf("customer-io: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("customer-io: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("customer-io API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			rejected := execution.RejectCredential(raw)
			return nil, &apiError{msg: rejected.Error(), status: resp.StatusCode, err: rejected}
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

// emitValue marshals a client-side value (receipts) and writes it to stdout
// (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("customer-io: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// apiMessage extracts Customer.io's error text from an error body. The App API
// returns either {"meta":{"error":"…"}} / {"meta":{"errors":["…"]}} or a bare
// {"errors":[{"detail":"…"}]} shape; fall back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Meta struct {
			Error  string   `json:"error"`
			Errors []string `json:"errors"`
		} `json:"meta"`
		Errors []struct {
			Detail  string `json:"detail"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Meta.Error != "":
			return e.Meta.Error
		case len(e.Meta.Errors) > 0:
			return strings.Join(e.Meta.Errors, "; ")
		case len(e.Errors) > 0:
			parts := make([]string, 0, len(e.Errors))
			for _, item := range e.Errors {
				if item.Detail != "" {
					parts = append(parts, item.Detail)
				} else if item.Message != "" {
					parts = append(parts, item.Message)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "; ")
			}
		}
	}
	return strings.TrimSpace(string(body))
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// metricsParams holds the shared reporting-window flags Customer.io's metric
// endpoints accept. period and steps are omitted when unset so the provider's
// own defaults apply; type narrows the metric series.
type metricsParams struct {
	period     string
	steps      int
	metricType string
}

// registerMetricsFlags wires --period / --steps / --type onto a metrics command.
func registerMetricsFlags(cmd *cobra.Command, m *metricsParams) {
	cmd.Flags().StringVar(&m.period, "period", "", "reporting period: hours|days|weeks|months")
	cmd.Flags().IntVar(&m.steps, "steps", 0, "number of periods to report (0 = provider default)")
	cmd.Flags().StringVar(&m.metricType, "type", "", "metric type filter (e.g. delivered, opened, clicked)")
}

// apply writes the metrics params into a query value set.
func (m metricsParams) apply(q url.Values) {
	if m.period != "" {
		q.Set("period", m.period)
	}
	if m.steps > 0 {
		q.Set("steps", strconv.Itoa(m.steps))
	}
	if m.metricType != "" {
		q.Set("type", m.metricType)
	}
}

// setIDType writes the id_type query param when it deviates from the "id"
// default. Customer.io person endpoints resolve the path id against this type.
func setIDType(q url.Values, idType string) {
	if t := strings.TrimSpace(idType); t != "" && t != "id" {
		q.Set("id_type", t)
	}
}
