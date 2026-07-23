package freshdesk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// client carries the resolved credential and base URL for one command
// invocation and performs the actual HTTP calls.
type client struct {
	apiKey string
	base   string
	svc    *Service
}

// authHeader builds the Freshdesk Basic-auth header: the API key as username
// with a dummy password ("X"), base64-encoded.
func (c *client) authHeader() string {
	raw := c.apiKey + ":" + dummyPassword
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// call performs one Freshdesk API request and returns the raw response body.
// 401/403 marks the credential rejected; 429 surfaces Freshdesk's Retry-After;
// any other non-2xx is a plain error carrying Freshdesk's description/errors.
func (c *client) call(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("freshdesk: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := c.base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("freshdesk: build request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.svc.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("freshdesk: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("freshdesk: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyError(resp, body)
	}
	return body, nil
}

// classifyError maps a non-2xx Freshdesk response to a typed error: 401/403
// reject the credential, 429 surfaces the Retry-After hint, everything else is
// a plain provider error.
func classifyError(resp *http.Response, body []byte) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return execution.RejectCredential(fmt.Errorf("freshdesk API error (HTTP %d): %s", resp.StatusCode, apiMessage(body)))
	case http.StatusTooManyRequests:
		retry := resp.Header.Get("Retry-After")
		if retry == "" {
			retry = "unknown"
		}
		return fmt.Errorf("freshdesk API error (HTTP 429): rate limited, retry after %s seconds: %s", retry, apiMessage(body))
	default:
		return fmt.Errorf("freshdesk API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
	}
}

// apiMessage extracts Freshdesk's error description (and nested field errors)
// from an error body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Description string `json:"description"`
		Message     string `json:"message"`
		Errors      []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		lead := e.Description
		if lead == "" {
			lead = e.Message
		}
		if len(e.Errors) > 0 {
			parts := make([]string, 0, len(e.Errors))
			for _, fe := range e.Errors {
				seg := fe.Message
				if fe.Field != "" {
					seg = fe.Field + ": " + seg
				}
				if fe.Code != "" {
					seg += " (" + fe.Code + ")"
				}
				parts = append(parts, seg)
			}
			detail := strings.Join(parts, "; ")
			if lead != "" {
				return lead + " [" + detail + "]"
			}
			return detail
		}
		if lead != "" {
			return lead
		}
	}
	return strings.TrimSpace(string(body))
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (c *client) emit(body []byte) error {
	if _, err := c.svc.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(c.svc.stdout(), "\n")
	return err
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("freshdesk: --%s is not valid JSON: %w", name, err)
	}
	return v, nil
}
