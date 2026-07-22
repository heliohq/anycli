package zuora

import (
	"context"
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

// apiError is a runtime / API error: a Zuora non-2xx response, an error
// envelope carried in a 2xx body, or a transport failure. It maps to exit code
// 1 and kind "api". status is the HTTP status (0 for transport/network
// failures); code is Zuora's error code when the envelope carries one. It wraps
// the underlying cause so errors.As for *credentialRejectedError still resolves
// through it.
type apiError struct {
	msg    string
	status int
	code   string
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client performs the Zuora client-credentials exchange and the authorized data
// calls. The bearer is minted lazily on the first data call and cached for the
// process lifetime — Zuora rate-limits token minting by IP and documents "each
// token should be used until it expires," and an anycli process is far
// shorter-lived than a token (~1h), so it never needs a refresh loop.
type client struct {
	baseURL  string
	clientID string
	secret   string
	hc       *http.Client
	token    string // cached bearer, empty until first exchange
}

// accessToken returns a bearer, minting one via the client_credentials grant on
// first use. Per Zuora's createToken reference the request is form-encoded with
// grant_type/client_id/client_secret in the BODY and NO authentication headers
// (do not set Authorization / apiAccessKeyId / apiSecretAccessKey). A 401 here
// is a rejected credential pair (wrong id/secret, or the pair belongs to a
// different data center than base_url).
func (c *client) accessToken(ctx context.Context) (string, error) {
	if c.token != "" {
		return c.token, nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.secret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zuora: build token request: %v", err), err: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zuora: token exchange: %v", err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zuora: read token response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", newAPIError(resp.StatusCode, body, "obtain access token")
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", &apiError{msg: fmt.Sprintf("zuora: decode token response: %v", err), err: err}
	}
	if strings.TrimSpace(tok.AccessToken) == "" {
		return "", &apiError{msg: "zuora: token response carried no access_token"}
	}
	c.token = tok.AccessToken
	return c.token, nil
}

// call performs one authorized Zuora REST request. query is appended when
// non-empty; payload (if non-nil) is JSON-encoded as the request body. A non-2xx
// surfaces the Zuora error envelope as an apiError carrying the HTTP status; a
// 2xx body that still carries an error envelope (Zuora returns some failures as
// 200 with success:false) is ALSO surfaced as an apiError — never passed through
// as a false success; a transport failure surfaces as an apiError with status 0.
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
			return nil, &apiError{msg: fmt.Sprintf("zuora: encode request: %v", mErr), err: mErr}
		}
		reqBody = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, full, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zuora: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zuora: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zuora: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp.StatusCode, body, fmt.Sprintf("%s %s", method, path))
	}
	// Guard silent-success-on-failure: some Zuora endpoints return HTTP 200
	// with an error envelope (success:false / Success:false / non-empty
	// reasons|Errors). Treat that as a failure with the same exit code as a
	// non-2xx, not a passthrough success body.
	if failed, code, message := failureFromBody(body); failed {
		return nil, &apiError{
			msg:    fmt.Sprintf("zuora: %s %s failed (HTTP %d): %s", method, path, resp.StatusCode, message),
			status: resp.StatusCode,
			code:   code,
			err:    fmt.Errorf("zuora %s %s: %s", method, path, message),
		}
	}
	return body, nil
}

// zuoraErrorEnvelope models the THREE distinct failure shapes Zuora returns,
// verified against developer.zuora.com, so detection is case-insensitive across
// both flavors and also covers the bare-message form:
//
//   - Standard /v1 resource reads (accounts, subscriptions, invoices, payments,
//     catalog) — EXCEPT Actions/CRUD: lowercase {success:false, reasons:[{code,
//     message}]} with an eight-digit numeric code.
//   - Actions (POST /v1/action/query is an Action) and CRUD: the SOAP-derived
//     capitalized {Success:false, Errors:[{Code,Message}]}.
//   - The Action_POSTquery reference documents query FAILURE as HTTP 400/401
//     with a bare {message:…} (no success flag at all).
//
// The two envelopes carry DIFFERENT code types on the wire — the lowercase
// resource form's reasons[].code is an eight-digit NUMBER, while the SOAP-derived
// capitalized Errors[].Code is a STRING enum (e.g. "INVALID_FIELD"). Both are
// decoded as json.RawMessage and rendered by rawCode so neither shape breaks the
// unmarshal.
type zuoraErrorEnvelope struct {
	SuccessLower *bool  `json:"success"`
	SuccessUpper *bool  `json:"Success"`
	Message      string `json:"message"`
	Reasons      []struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"reasons"`
	Errors []struct {
		Code    json.RawMessage `json:"Code"`
		Message string          `json:"Message"`
	} `json:"Errors"`
}

// rawCode renders a JSON code token (a bare number like 50000040 or a quoted
// string like "INVALID_FIELD") as a plain string, trimming surrounding quotes.
func rawCode(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal(raw, &unquoted); err == nil {
			return unquoted
		}
	}
	if s == "null" {
		return ""
	}
	return s
}

// failureFromBody reports whether a response body carries any of Zuora's error
// envelopes, and extracts a representative code + human message. It is used for
// the 2xx-with-error-envelope guard; newAPIError reuses it for non-2xx bodies.
// A body that is not JSON, or JSON with no error signal, returns failed=false.
func failureFromBody(body []byte) (failed bool, code string, message string) {
	var env zuoraErrorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return false, "", ""
	}
	explicitFalse := (env.SuccessLower != nil && !*env.SuccessLower) ||
		(env.SuccessUpper != nil && !*env.SuccessUpper)
	switch {
	case len(env.Reasons) > 0:
		return true, rawCode(env.Reasons[0].Code), firstNonEmpty(env.Reasons[0].Message, "request failed")
	case len(env.Errors) > 0:
		return true, rawCode(env.Errors[0].Code), firstNonEmpty(env.Errors[0].Message, "request failed")
	case explicitFalse:
		return true, "", firstNonEmpty(env.Message, "request failed")
	default:
		return false, "", ""
	}
}

// newAPIError builds the classified apiError for a Zuora non-2xx response. It
// prefers the parsed envelope's code/message (any of the three forms); a 401 on
// the token exchange or a REST call is a credential rejection so the engine can
// surface a reconnect prompt. A body with no recognizable envelope (e.g. the
// bare {message} query-failure form, or a raw string) still renders its text.
func newAPIError(status int, body []byte, action string) error {
	failed, code, message := failureFromBody(body)
	if !failed {
		// Fall back to a bare {"message": …} (query-failure form) or the raw
		// body text when no structured envelope matched.
		message = bareMessage(body)
	}
	base := fmt.Sprintf("zuora: %s failed (HTTP %d): %s", action, status, message)
	inner := fmt.Errorf("%s", base)
	if status == http.StatusUnauthorized {
		return &apiError{msg: base, status: status, code: code, err: execution.RejectCredential(inner)}
	}
	return &apiError{msg: base, status: status, code: code, err: inner}
}

// bareMessage extracts a top-level {"message": …} (Zuora's Action_POSTquery
// failure form), falling back to the trimmed raw body when absent.
func bareMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && strings.TrimSpace(e.Message) != "" {
		return strings.TrimSpace(e.Message)
	}
	return strings.TrimSpace(string(body))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
