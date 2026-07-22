package later

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const maxErrorBodyBytes = 8 << 10

// reportingClient owns one Later Influence reporting session: it lazily mints a
// short-lived JWT from the client-credentials pair and reuses it across the
// (single) command invocation, re-minting once on a 401.
type reportingClient struct {
	svc          *Service
	clientID     string
	clientSecret string
	jwt          string
}

// tokenRequest is the /oauth/token client-credentials body (JSON, camelCase
// per Later's docs — not form-encoded).
type tokenRequest struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

type tokenResponse struct {
	JWT string `json:"jwt"`
}

// mint exchanges the client-credentials pair for a JWT (POST /oauth/token). A
// non-2xx here is a credential problem: the pair is the credential, so a bad
// pair is classified as rejected.
func (c *reportingClient) mint(ctx context.Context) error {
	payload, err := json.Marshal(tokenRequest{ClientID: c.clientID, ClientSecret: c.clientSecret})
	if err != nil {
		return fmt.Errorf("later: encode token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.svc.baseURL()+"/oauth/token", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("later: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.svc.client().Do(req)
	if err != nil {
		return fmt.Errorf("later: POST /oauth/token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("later: read token response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return newTokenError(resp.StatusCode, body)
	}
	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("later: decode token response: %w", err)
	}
	if parsed.JWT == "" {
		return execution.RejectCredential(fmt.Errorf("later: /oauth/token returned no jwt"))
	}
	c.jwt = parsed.JWT
	return nil
}

// get performs one authenticated GET, minting the JWT on first use and
// re-minting exactly once if the data call returns 401 (expired token). A
// persistent 401 after a fresh mint marks the credential rejected.
func (c *reportingClient) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if c.jwt == "" {
		if err := c.mint(ctx); err != nil {
			return nil, err
		}
	}
	body, status, err := c.do(ctx, path, query)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		// Token likely expired mid-session: re-mint once and retry.
		if err := c.mint(ctx); err != nil {
			return nil, err
		}
		body, status, err = c.do(ctx, path, query)
		if err != nil {
			return nil, err
		}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, newAPIError(status, body)
	}
	return body, nil
}

// do issues one GET with the current JWT and returns the body and status
// without classifying non-2xx (the caller owns the 401 re-mint decision).
func (c *reportingClient) do(ctx context.Context, path string, query url.Values) ([]byte, int, error) {
	requestURL := c.svc.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("later: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.jwt)
	req.Header.Set("Accept", "application/json")

	resp, err := c.svc.client().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("later: GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("later: read response: %w", err)
	}
	return body, resp.StatusCode, nil
}

// emit writes the provider JSON response to stdout verbatim (+ newline),
// matching the passthrough convention of the other built-in services.
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
