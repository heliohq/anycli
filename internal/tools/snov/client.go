package snov

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// tokenPath is the two-legged client_credentials exchange endpoint.
const tokenPath = "/v1/oauth/access_token"

// ensureToken exchanges the account's client_id/client_secret for a Bearer
// access token, caching it for the invocation. A rejection of the credentials
// (the only inputs are the two secrets) is classified as CredentialRejected so
// the Helio token gateway learns the stored secrets are bad. The exchange runs
// at most once per process — every API call funnels through here.
func (s *Service) ensureToken(ctx context.Context, creds clientCreds) (string, error) {
	if s.token != "" {
		return s.token, nil
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", creds.clientID)
	form.Set("client_secret", creds.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+tokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("snov: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("snov: token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("snov: read token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("snov token exchange failed (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		// The client_credentials pair is the sole input; a 4xx here is a bad or
		// revoked secret, not a transient fault — reject the credential so the
		// gateway surfaces a reconnect. 5xx stays a plain (retryable) error.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", execution.RejectCredential(apiErr)
		}
		return "", apiErr
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("snov: decode token response: %w", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", execution.RejectCredential(fmt.Errorf("snov token exchange returned no access_token"))
	}
	s.token = out.AccessToken
	return s.token, nil
}

// callV1 performs a synchronous Snov v1 request. v1 methods authenticate with
// the access_token as a request parameter (query for GET, form body for POST)
// and return their result directly. Returns the raw response body.
func (s *Service) callV1(ctx context.Context, creds clientCreds, method, path string, params url.Values) ([]byte, error) {
	token, err := s.ensureToken(ctx, creds)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("access_token", token)

	requestURL := s.baseURL() + path
	var reqBody io.Reader
	if method == http.MethodGet {
		requestURL += "?" + params.Encode()
	} else {
		reqBody = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("snov: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return s.do(req, method, path)
}

// callV2Start performs a Snov v2 asynchronous `start` request: Bearer auth with
// a JSON body. Returns the raw start response (carrying task_hash and, for
// domain-search, a links.result URL).
func (s *Service) callV2Start(ctx context.Context, creds clientCreds, path string, payload any) ([]byte, error) {
	token, err := s.ensureToken(ctx, creds)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("snov: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("snov: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return s.do(req, http.MethodPost, path)
}

// startResponse is the minimal shape the poll loop reads off a v2 start reply:
// the task hash and (domain-search only) a full pollable result URL.
type startResponse struct {
	TaskHash string `json:"task_hash"`
	Meta     struct {
		TaskHash string `json:"task_hash"`
	} `json:"meta"`
	Links struct {
		Result string `json:"result"`
	} `json:"links"`
}

// taskHash returns the task hash from either the top-level or meta location.
func (r startResponse) taskHash() string {
	if strings.TrimSpace(r.TaskHash) != "" {
		return r.TaskHash
	}
	return strings.TrimSpace(r.Meta.TaskHash)
}

// startAndPoll runs a v2 start call, then blocks polling the matching result
// endpoint until the task completes (or the poll budget is exhausted), and
// returns the final result body. resultPath is the result endpoint used when
// the start response carries no links.result URL; the hash is appended as a
// task_hash query parameter. This hides Snov's async task model behind one
// synchronous command.
func (s *Service) startAndPoll(ctx context.Context, creds clientCreds, startPath, resultPath string, payload any, timeout time.Duration) ([]byte, error) {
	startBody, err := s.callV2Start(ctx, creds, startPath, payload)
	if err != nil {
		return nil, err
	}
	var start startResponse
	if err := json.Unmarshal(startBody, &start); err != nil {
		return nil, fmt.Errorf("snov: decode start response: %w", err)
	}
	resultURL := strings.TrimSpace(start.Links.Result)
	if resultURL == "" {
		hash := start.taskHash()
		if hash == "" {
			return nil, fmt.Errorf("snov: start response carried no task_hash or result URL: %s", strings.TrimSpace(string(startBody)))
		}
		resultURL = s.baseURL() + resultPath + "?task_hash=" + url.QueryEscape(hash)
	}
	return s.poll(ctx, creds, resultURL, timeout)
}

// poll GETs resultURL (Bearer) until the task's status is no longer in
// progress, respecting the configured interval and an overall timeout.
func (s *Service) poll(ctx context.Context, creds clientCreds, resultURL string, timeout time.Duration) ([]byte, error) {
	token, err := s.ensureToken(ctx, creds)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
		if err != nil {
			return nil, fmt.Errorf("snov: build result request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		body, err := s.do(req, http.MethodGet, resultURL)
		if err != nil {
			return nil, err
		}
		if !inProgress(body) {
			return body, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("snov: task did not complete within %s; retry or raise --timeout", timeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.pollInterval()):
		}
	}
}

// inProgress reports whether a v2 result body signals the task is still
// running. Snov reports the state in a `status` field at the top level or under
// `meta`; anything other than an explicit in-progress marker is treated as
// terminal so the final payload is returned.
func inProgress(body []byte) bool {
	var probe struct {
		Status string `json:"status"`
		Meta   struct {
			Status string `json:"status"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(probe.Status))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(probe.Meta.Status))
	}
	switch status {
	case "in progress", "in_progress", "inprogress", "pending", "processing":
		return true
	default:
		return false
	}
}

// do executes a prepared request and returns the response body. A 401/403 marks
// the credential rejected; any other non-2xx is a plain error carrying Snov's
// message.
func (s *Service) do(req *http.Request, method, path string) ([]byte, error) {
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("snov: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("snov: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("snov API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
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

// apiMessage extracts a human-readable error from a Snov error body, falling
// back to the raw body. Snov surfaces failures under `message` or `error`
// (sometimes an array of validation messages).
func apiMessage(body []byte) string {
	var e struct {
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
		Errors  json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if strings.TrimSpace(e.Message) != "" {
			return e.Message
		}
		if len(e.Error) > 0 && string(e.Error) != "null" {
			return strings.Trim(string(e.Error), `"`)
		}
		if len(e.Errors) > 0 && string(e.Errors) != "null" {
			return string(e.Errors)
		}
	}
	return strings.TrimSpace(string(body))
}
