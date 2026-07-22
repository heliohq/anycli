package billcom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error (bad flag, missing --data, invalid
// JSON). It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a BILL non-2xx response, a transport
// failure, or a login failure. It maps to exit code 1 and kind "api". status is
// the HTTP status (0 for transport/network failures). It wraps the cause so
// errors.As for the credential-rejected sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client carries the resolved credentials and per-invocation session state and
// performs the BILL login->call session dance. One client is built per
// invocation, so login happens once (lazily, on the first API call) and the
// minted sessionId is reused across a single command.
type client struct {
	hc         *http.Client
	baseURL    string // v3 gateway base incl. /connect/v3
	loginV2URL string // full v2 login URL (…/api/v2/Login.json)
	devKey     string
	username   string
	password   string
	orgID      string
	authMode   string // "" / "v3" (default) or "sync_token"
	out        io.Writer
	err        io.Writer

	session string // cached sessionId (empty until first login)
}

// newClient resolves the credential env map into a client, deriving the v3/v2
// base URLs from BILLCOM_ENV (prod default; "stage"/"sandbox" -> stage host)
// unless the Service overrides them (tests). Missing any of the four required
// credential fields is a fail-fast config error.
func (s *Service) newClient(env map[string]string) (*client, error) {
	devKey := env[EnvDevKey]
	username := env[EnvUsername]
	password := env[EnvPassword]
	orgID := env[EnvOrgID]

	var missing []string
	if devKey == "" {
		missing = append(missing, "dev_key ("+EnvDevKey+")")
	}
	if username == "" {
		missing = append(missing, "username ("+EnvUsername+")")
	}
	if password == "" {
		missing = append(missing, "password ("+EnvPassword+")")
	}
	if orgID == "" {
		missing = append(missing, "organization_id ("+EnvOrgID+")")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("bill-com: missing required credential(s): %s", strings.Join(missing, ", "))
	}

	envName := strings.ToLower(strings.TrimSpace(env[EnvEnv]))
	base := s.BaseURL
	if base == "" {
		base = v3Base(envName)
	}
	v2Login := ""
	if s.LoginV2BaseURL != "" {
		v2Login = strings.TrimRight(s.LoginV2BaseURL, "/") + "/api/v2/Login.json"
	} else {
		v2Login = v2Base(envName) + "/api/v2/Login.json"
	}

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}

	return &client{
		hc:         hc,
		baseURL:    strings.TrimRight(base, "/"),
		loginV2URL: v2Login,
		devKey:     devKey,
		username:   username,
		password:   password,
		orgID:      orgID,
		authMode:   strings.ToLower(strings.TrimSpace(env[EnvAuthMode])),
		out:        s.stdout(),
		err:        s.stderr(),
	}, nil
}

// v3Base returns the v3 gateway base for the given normalized env name.
func v3Base(envName string) string {
	host := "prod"
	if envName == "stage" || envName == "sandbox" {
		host = "stage"
	}
	return fmt.Sprintf("https://gateway.%s.bill.com/connect/v3", host)
}

// v2Base returns the v2 API host base (no path) for the given normalized env.
func v2Base(envName string) string {
	if envName == "stage" || envName == "sandbox" {
		return "https://api-stage.bill.com"
	}
	return "https://api.bill.com"
}

// login mints a fresh sessionId and caches it on the client. The endpoint
// depends on the auth mode: the AP & AR sync token must sign in with the BILL
// v2 login operation (POST /api/v2/Login.json, form-encoded), while the raw /
// Accountant-Console credential uses the v3 login (POST {base}/login, JSON).
// Either way the returned sessionId rides as a header on the v3 resource calls.
func (c *client) login(ctx context.Context) error {
	if c.authMode == authModeSyncToken {
		return c.loginV2(ctx)
	}
	return c.loginV3(ctx)
}

// loginV3 signs in via the Connect v3 JSON login endpoint.
func (c *client) loginV3(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"devKey":         c.devKey,
		"username":       c.username,
		"password":       c.password,
		"organizationId": c.orgID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", bytes.NewReader(body))
	if err != nil {
		return &apiError{msg: fmt.Sprintf("bill-com: build v3 login: %v", err), err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	respBody, status, err := c.doHTTP(req)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return c.loginFailure(status, respBody)
	}
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || out.SessionID == "" {
		return &apiError{msg: fmt.Sprintf("bill-com: v3 login returned no sessionId: %s", string(respBody)), status: status}
	}
	c.session = out.SessionID
	return nil
}

// loginV2 signs in via the BILL v2 login operation for the AP & AR sync token.
func (c *client) loginV2(ctx context.Context) error {
	form := url.Values{}
	form.Set("userName", c.username)
	form.Set("password", c.password)
	form.Set("orgId", c.orgID)
	form.Set("devKey", c.devKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.loginV2URL, strings.NewReader(form.Encode()))
	if err != nil {
		return &apiError{msg: fmt.Sprintf("bill-com: build v2 login: %v", err), err: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	respBody, status, err := c.doHTTP(req)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return c.loginFailure(status, respBody)
	}
	var out struct {
		ResponseStatus  int    `json:"response_status"`
		ResponseMessage string `json:"response_message"`
		ResponseData    struct {
			SessionID string `json:"sessionId"`
		} `json:"response_data"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return &apiError{msg: fmt.Sprintf("bill-com: v2 login decode: %v", err), status: status, err: err}
	}
	if out.ResponseStatus != 0 || out.ResponseData.SessionID == "" {
		msg := out.ResponseMessage
		if msg == "" {
			msg = string(respBody)
		}
		return c.loginFailure(status, []byte(msg))
	}
	c.session = out.ResponseData.SessionID
	return nil
}

// loginFailure builds a credential-classified apiError for a rejected login.
func (c *client) loginFailure(status int, body []byte) error {
	raw := fmt.Errorf("bill-com login failed (HTTP %d): %s", status, strings.TrimSpace(string(body)))
	// A rejected login is a credential problem: mark the credential stale so
	// the host can prompt for a reconnect.
	return &apiError{msg: raw.Error(), status: status, err: execution.RejectCredential(raw)}
}

// do performs one authenticated v3 resource request, logging in first when no
// session is cached. On an expired-session 401 it re-logs-in once and retries —
// but only for idempotent GETs; a POST is attempted once (no blind retry of a
// create). query and payload are optional.
func (c *client) do(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	if c.session == "" {
		if err := c.login(ctx); err != nil {
			return nil, err
		}
	}
	body, status, err := c.roundTrip(ctx, method, path, query, payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized && method == http.MethodGet {
		// Session likely expired (35-min / 48-h inactivity). Re-login once
		// and retry the idempotent GET.
		c.session = ""
		if lerr := c.login(ctx); lerr != nil {
			return nil, lerr
		}
		body, status, err = c.roundTrip(ctx, method, path, query, payload)
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status > 299 {
		raw := fmt.Errorf("bill-com API error (HTTP %d): %s", status, apiMessage(body))
		classified := classifyCredentialError(status, raw)
		return nil, &apiError{msg: classified.Error(), status: status, err: classified}
	}
	return body, nil
}

// roundTrip issues one request with the devKey + sessionId headers and returns
// the raw body and status.
func (c *client) roundTrip(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, int, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, &apiError{msg: fmt.Sprintf("bill-com: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, 0, &apiError{msg: fmt.Sprintf("bill-com: build request: %v", err), err: err}
	}
	req.Header.Set("devKey", c.devKey)
	req.Header.Set("sessionId", c.session)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	body, status, err := c.doHTTP(req)
	return body, status, err
}

// doHTTP executes a prepared request and reads the body, wrapping transport /
// read failures as apiErrors with status 0.
func (c *client) doHTTP(req *http.Request) ([]byte, int, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, &apiError{msg: fmt.Sprintf("bill-com: %s %s: %v", req.Method, req.URL.Path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, &apiError{msg: fmt.Sprintf("bill-com: read response: %v", err), err: err}
	}
	return body, resp.StatusCode, nil
}

// classifyCredentialError marks 401 responses as credential rejections so the
// host can flag the connection for reconnect.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts BILL's error message from an error body, falling back to
// the raw body. BILL v3 errors carry {"errors":[{"code","message"}]}.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if len(e.Errors) > 0 && (e.Errors[0].Code != "" || e.Errors[0].Message != "") {
			return fmt.Sprintf("%s: %s", e.Errors[0].Code, e.Errors[0].Message)
		}
		if e.Message != "" {
			return e.Message
		}
	}
	return strings.TrimSpace(string(body))
}
