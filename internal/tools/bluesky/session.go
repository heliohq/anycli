package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxErrorBodyBytes = 8 << 10

// session holds the per-process authenticated state. The service opens exactly
// one session at first use (createSession is cheap and the docs bless "a single
// session" for one-off requests), caches the access token + identity in memory,
// and re-establishes once if an access token expires mid-process.
type session struct {
	svc        *Service
	host       string
	identifier string
	password   string

	accessJwt string
	did       string
	handle    string
	ready     bool
}

type createSessionResponse struct {
	AccessJwt  string `json:"accessJwt"`
	RefreshJwt string `json:"refreshJwt"`
	Handle     string `json:"handle"`
	DID        string `json:"did"`
}

// ensure opens the session if it is not already established.
func (se *session) ensure(ctx context.Context) error {
	if se.ready {
		return nil
	}
	return se.establish(ctx)
}

func (se *session) establish(ctx context.Context) error {
	payload := map[string]string{
		"identifier": se.identifier,
		"password":   se.password,
	}
	body, err := se.rawCall(ctx, http.MethodPost, "com.atproto.server.createSession", nil, payload, "")
	if err != nil {
		return err
	}
	var resp createSessionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("bluesky: decode session: %w", err)
	}
	if resp.AccessJwt == "" || resp.DID == "" {
		return fmt.Errorf("bluesky: session response missing accessJwt or did")
	}
	se.accessJwt = resp.AccessJwt
	se.did = resp.DID
	se.handle = resp.Handle
	se.ready = true
	return nil
}

// call issues an authenticated XRPC request, opening the session first. On an
// ExpiredToken response it re-establishes the session once and retries.
func (se *session) call(ctx context.Context, method, nsid string, query url.Values, payload any) ([]byte, error) {
	if err := se.ensure(ctx); err != nil {
		return nil, err
	}
	body, err := se.rawCall(ctx, method, nsid, query, payload, se.accessJwt)
	if err != nil && isExpiredToken(err) {
		if reErr := se.establish(ctx); reErr != nil {
			return nil, reErr
		}
		return se.rawCall(ctx, method, nsid, query, payload, se.accessJwt)
	}
	return body, err
}

// rawCall performs one XRPC HTTP round trip. A non-empty token is sent as a
// plain Bearer credential (app-password sessions are not DPoP-bound).
func (se *session) rawCall(ctx context.Context, method, nsid string, query url.Values, payload any, token string) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("bluesky: encode request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	requestURL := se.host + "/xrpc/" + nsid
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("bluesky: build request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return se.do(req, token)
}

// uploadBlob sends raw image bytes to com.atproto.repo.uploadBlob with the
// image's own content type and returns the parsed blob reference to embed.
func (se *session) uploadBlob(ctx context.Context, data []byte, contentType string) (json.RawMessage, error) {
	if err := se.ensure(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, se.host+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("bluesky: build upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+se.accessJwt)
	req.Header.Set("Content-Type", contentType)

	body, err := se.do(req, se.accessJwt)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Blob json.RawMessage `json:"blob"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bluesky: decode upload response: %w", err)
	}
	if len(resp.Blob) == 0 {
		return nil, fmt.Errorf("bluesky: upload response has no blob")
	}
	return resp.Blob, nil
}

func (se *session) do(req *http.Request, token string) ([]byte, error) {
	resp, err := se.svc.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("bluesky: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bluesky: read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, body, token)
	}
	return body, nil
}

// providerError is the XRPC error envelope: {"error":"Name","message":"..."}.
type providerError struct {
	Name    string
	Message string
	Status  int
}

func (e *providerError) Error() string {
	hint := ""
	switch e.Status {
	case http.StatusUnauthorized:
		hint = "; identifier or app password is invalid — reconnect Bluesky"
	case http.StatusTooManyRequests:
		hint = "; rate limit exceeded — retry after the provider reset window"
	}
	detail := e.Message
	if detail == "" {
		detail = e.Name
	}
	return fmt.Sprintf("bluesky API error (HTTP %d): %s%s", e.Status, detail, hint)
}

func newAPIError(status int, body []byte, token string) error {
	var envelope struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &envelope)

	name := envelope.Error
	message := envelope.Message
	if name == "" && message == "" {
		raw := strings.TrimSpace(string(body))
		if token != "" {
			raw = strings.ReplaceAll(raw, token, "[REDACTED]")
		}
		if len(raw) > maxErrorBodyBytes {
			raw = raw[:maxErrorBodyBytes] + "…"
		}
		message = raw
	}

	apiErr := &providerError{Name: name, Message: message, Status: status}
	return classifyCredentialError(status, name, apiErr)
}
