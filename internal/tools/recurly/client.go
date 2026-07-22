package recurly

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

// apiVersionAccept is the mandatory Accept header pinning the V3 API version.
// Recurly requires an explicit version or it returns 406; v2021-02-25 is the
// current stable JSON version (client-library 4.x line).
const apiVersionAccept = "application/vnd.recurly.v2021-02-25"

// Regional hosts. The data center is chosen by the merchant's Recurly site, not
// encoded in the key, so it is supplied out-of-band via RECURLY_REGION.
const (
	hostUS = "https://v3.recurly.com"
	hostEU = "https://v3.eu.recurly.com"
)

// Env vars the credential binding injects (definitions/tools/recurly.json).
const (
	EnvKey    = "RECURLY_API_KEY"
	EnvRegion = "RECURLY_REGION"
)

// hostForRegion maps a region hint to a Recurly host. Only "eu" selects the EU
// data center; every other value (including empty) defaults to US.
func hostForRegion(region string) string {
	if strings.EqualFold(strings.TrimSpace(region), "eu") {
		return hostEU
	}
	return hostUS
}

// usageError is a parameter/usage error (bad flag, missing arg, invalid JSON).
// It maps to exit code 2 and error kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime/API error: a Recurly non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport failures). It wraps its cause so errors.As for a credential
// rejection resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// baseURL is the effective host for a call: an explicit test override wins,
// otherwise the region-selected host.
func (s *Service) baseURL(region string) string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	return hostForRegion(region)
}

// call performs one Recurly request: Basic auth with the private key as the
// username and a blank password, the mandatory version Accept header, and JSON
// content when a payload is present. A non-2xx surfaces the typed error
// envelope as an apiError carrying the HTTP status (401/invalid_api_key are
// classified as credential rejections); a transport failure is an apiError with
// status 0.
func (s *Service) call(ctx context.Context, key, region, method, path string, query url.Values, payload []byte) ([]byte, error) {
	u := s.baseURL(region) + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("recurly: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key+":")))
	req.Header.Set("Accept", apiVersionAccept)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("recurly: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("recurly: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, s.classifyError(resp, body)
	}
	return body, nil
}

// recurlyErrorEnvelope is Recurly's error body: {"error":{"type","message",…}}.
type recurlyErrorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// classifyError renders a non-2xx response into an apiError. It extracts the
// typed error envelope, echoes Retry-After on 429, and marks authentication
// failures as credential rejections so the engine can invalidate the key.
func (s *Service) classifyError(resp *http.Response, body []byte) error {
	etype, emsg := parseErrorEnvelope(body)
	msg := fmt.Sprintf("recurly API error (HTTP %d)", resp.StatusCode)
	if etype != "" || emsg != "" {
		msg = fmt.Sprintf("recurly API error (HTTP %d): %s: %s", resp.StatusCode, etype, emsg)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			msg += fmt.Sprintf(" (retry after %ss)", ra)
		}
	}
	base := &apiError{msg: msg, status: resp.StatusCode}
	if isCredentialFailure(resp.StatusCode, etype) {
		base.err = execution.RejectCredential(fmt.Errorf("%s", msg))
		return base
	}
	return base
}

// parseErrorEnvelope pulls the type and message from a Recurly error body,
// tolerating a non-JSON body (returns the raw text as the message).
func parseErrorEnvelope(body []byte) (etype, message string) {
	var env recurlyErrorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && (env.Error.Type != "" || env.Error.Message != "") {
		return env.Error.Type, env.Error.Message
	}
	return "", strings.TrimSpace(string(body))
}

// isCredentialFailure reports whether a failure is an explicit rejection of the
// key (a 401, or Recurly's invalid_api_key/unauthorized error types) rather
// than a permission, rate-limit, or resource error that leaves the key valid.
func isCredentialFailure(status int, etype string) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	switch etype {
	case "unauthorized", "invalid_api_key", "invalid_token":
		return true
	}
	return false
}

// listEnvelope is the provider-neutral list shape emitted for every `list` leaf.
// Recurly's native list body is {"object":"list","has_more":…,"next":"<url>",
// "data":[…]}; next is reduced to the bare cursor so it round-trips to --cursor.
type listEnvelope struct {
	Data    json.RawMessage `json:"data"`
	HasMore bool            `json:"has_more"`
	Next    string          `json:"next,omitempty"`
}

// toListEnvelope reshapes a Recurly list body into listEnvelope, extracting the
// cursor from the next URL. A body that is not a list (missing data) is returned
// verbatim so unexpected shapes are never silently dropped.
func toListEnvelope(body []byte) ([]byte, error) {
	var raw struct {
		Data    json.RawMessage `json:"data"`
		HasMore bool            `json:"has_more"`
		Next    string          `json:"next"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.Data == nil {
		return body, nil
	}
	env := listEnvelope{Data: raw.Data, HasMore: raw.HasMore, Next: cursorFromNext(raw.Next)}
	return json.Marshal(env)
}

// cursorFromNext extracts the `cursor` query value from Recurly's next URL. A
// value that is already a bare cursor (no URL) is returned unchanged; an
// unparseable value falls back to the raw string.
func cursorFromNext(next string) string {
	if next == "" {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil {
		return next
	}
	if c := u.Query().Get("cursor"); c != "" {
		return c
	}
	return next
}
