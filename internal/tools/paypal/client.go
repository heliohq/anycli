package paypal

import (
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

// usageError is a parameter / usage error: a bad flag combination, missing
// required flag, or an invalid value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a PayPal non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client performs the PayPal client-credentials exchange and the authorized
// data calls. The bearer is minted lazily on the first data call and cached for
// the process lifetime (PayPal tokens live ~9h; an anycli process is far
// shorter-lived, so it never needs a refresh loop).
type client struct {
	baseURL  string
	clientID string
	secret   string
	hc       *http.Client
	token    string // cached bearer, empty until first exchange
}

// accessToken returns a bearer, minting one via the client_credentials grant on
// first use. HTTP Basic authenticates the confidential client; the body is the
// standard form grant. A 401 here is a rejected credential pair (wrong
// id/secret, or wrong environment for the pair).
func (c *client) accessToken(ctx context.Context) (string, error) {
	if c.token != "" {
		return c.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("paypal: build token request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(c.clientID, c.secret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("paypal: token exchange: %v", err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("paypal: read token response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", newAPIError(resp.StatusCode, body, "obtain access token")
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", &apiError{msg: fmt.Sprintf("paypal: decode token response: %v", err), err: err}
	}
	if strings.TrimSpace(tok.AccessToken) == "" {
		return "", &apiError{msg: "paypal: token response carried no access_token"}
	}
	c.token = tok.AccessToken
	return c.token, nil
}

// call performs one authorized PayPal REST request. query is appended when
// non-empty; payload (if non-nil) is JSON-encoded as the request body. A non-2xx
// surfaces the PayPal error body (name + message) as an apiError carrying the
// HTTP status; a transport failure surfaces as an apiError with status 0.
func (c *client) call(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, mErr := json.Marshal(payload)
		if mErr != nil {
			return nil, &apiError{msg: fmt.Sprintf("paypal: encode request: %v", mErr), err: mErr}
		}
		reqBody = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, full, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("paypal: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("paypal: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("paypal: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp.StatusCode, body, fmt.Sprintf("%s %s", method, path))
	}
	return body, nil
}

// newAPIError builds the classified apiError for a PayPal non-2xx response,
// distinguishing the failure classes an AI teammate must react to differently:
//   - 401 → credential rejection (bad/expired pair, or wrong environment) →
//     RejectCredential so the engine can surface a reconnect prompt.
//   - 403 → the token is valid but the REST app/account lacks the feature scope
//     (Transaction Search / Invoicing not enabled) — NOT an auth retry.
//   - 422 → PayPal validation (UNPROCESSABLE_ENTITY): the request shape/values
//     are wrong.
//   - 429 → rate limited.
func newAPIError(status int, body []byte, action string) error {
	base := fmt.Sprintf("paypal: %s failed (HTTP %d): %s", action, status, apiMessage(body))
	switch status {
	case http.StatusUnauthorized:
		return &apiError{msg: base, status: status, err: execution.RejectCredential(fmt.Errorf("%s", base))}
	case http.StatusForbidden:
		msg := base + " (this PayPal REST app/account lacks the feature for this call — enable Invoicing and/or Transaction Search on the app and account)"
		return &apiError{msg: msg, status: status, err: fmt.Errorf("%s", msg)}
	case http.StatusTooManyRequests:
		msg := base + " (rate limited — retry after a short delay)"
		return &apiError{msg: msg, status: status, err: fmt.Errorf("%s", msg)}
	default:
		return &apiError{msg: base, status: status, err: fmt.Errorf("%s", base)}
	}
}

// apiMessage extracts PayPal's error name + message from an error body, adding
// the first issue detail when present, and falls back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Name    string `json:"name"`
		Message string `json:"message"`
		Error   string `json:"error"`             // token endpoint uses error/error_description
		ErrDesc string `json:"error_description"` //nolint:tagliatelle
		Details []struct {
			Issue       string `json:"issue"`
			Description string `json:"description"`
		} `json:"details"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Name != "" || e.Message != "":
			out := strings.TrimSpace(e.Name + ": " + e.Message)
			if len(e.Details) > 0 && e.Details[0].Issue != "" {
				out += fmt.Sprintf(" [%s: %s]", e.Details[0].Issue, e.Details[0].Description)
			}
			return out
		case e.Error != "" || e.ErrDesc != "":
			return strings.TrimSpace(e.Error + ": " + e.ErrDesc)
		}
	}
	return strings.TrimSpace(string(body))
}

// basicAuth builds the HTTP Basic credential from the client id + secret.
func basicAuth(clientID, secret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + secret))
}
