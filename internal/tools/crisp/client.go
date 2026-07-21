package crisp

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
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: a missing required flag, a bad enum
// value, or a missing --website. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Crisp {error:true} envelope, a non-2xx
// status, or a transport failure. It maps to exit code 1 and kind "api". status
// is the HTTP status (0 for transport failures). It wraps the underlying cause
// so errors.As for the credential-rejection marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// crispEnvelope is Crisp's uniform {error, reason, data} response wrapper.
type crispEnvelope struct {
	Error  bool            `json:"error"`
	Reason string          `json:"reason"`
	Data   json.RawMessage `json:"data"`
}

// call performs one Crisp API request with Basic keypair auth and the constant
// website-tier header, then unwraps the Crisp envelope. On success it returns
// the inner data payload. A non-2xx status or an error:true body becomes an
// apiError; a 401 or invalid_session reason is additionally marked as a
// credential rejection.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) (json.RawMessage, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("crisp: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("crisp: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("X-Crisp-Tier", tierWebsite)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("crisp: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("crisp: read response: %v", err), err: err}
	}

	var env crispEnvelope
	_ = json.Unmarshal(raw, &env) // best-effort; failure handled below

	failed := resp.StatusCode < 200 || resp.StatusCode > 299 || env.Error
	if failed {
		reason := env.Reason
		if reason == "" {
			if trimmed := strings.TrimSpace(string(raw)); trimmed != "" {
				reason = trimmed
			} else {
				reason = http.StatusText(resp.StatusCode)
			}
		}
		base := fmt.Errorf("crisp API error (HTTP %d): %s", resp.StatusCode, reason)
		aerr := &apiError{msg: base.Error(), status: resp.StatusCode, err: base}
		if resp.StatusCode == http.StatusUnauthorized || reason == "invalid_session" {
			aerr.err = execution.RejectCredential(base)
		}
		return nil, aerr
	}
	return env.Data, nil
}

// emit writes the success envelope {"data": <crisp data>, "meta": {...}} to
// stdout (+ newline). A nil data payload is rendered as JSON null.
func (s *Service) emit(data json.RawMessage, meta map[string]any) error {
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	payload := struct {
		Data json.RawMessage `json:"data"`
		Meta map[string]any  `json:"meta"`
	}{Data: data, Meta: meta}
	b, err := json.Marshal(payload)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("crisp: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// websiteFlag reads and validates the required global --website flag.
func websiteFlag(cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString("website")
	if strings.TrimSpace(id) == "" {
		return "", &usageError{msg: "--website is required: the Crisp website_id (find it in your Crisp dashboard URL or Settings)"}
	}
	return id, nil
}

// requireFlag returns the flag value or a usageError naming the missing flag.
func requireFlag(cmd *cobra.Command, name string) (string, error) {
	v, _ := cmd.Flags().GetString(name)
	if strings.TrimSpace(v) == "" {
		return "", &usageError{msg: fmt.Sprintf("--%s is required", name)}
	}
	return v, nil
}
