package hunter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// itoa renders an int as a base-10 query value.
func itoa(n int) string { return strconv.Itoa(n) }

// apiError marks a failure that reached the provider layer (network, transport,
// or a non-2xx HTTP response). Execute maps it to exit 1 (runtime/API failure);
// every other error escaping a command — bad --json flags, missing required
// flags, unknown subcommands — is a usage error and maps to exit 2.
type apiError struct{ msg string }

func (e *apiError) Error() string { return e.msg }

// headerAPIKey is the auth header Hunter accepts; the key is never sent as the
// api_key query parameter so it cannot leak into request logs or URLs.
const headerAPIKey = "X-API-KEY"

// call performs one Hunter API request with X-API-KEY auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is a
// plain error carrying Hunter's errors[0] details/id. A 202 is a success
// passthrough (Email Verifier still running), not an error.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("hunter: encode request: %v", err)}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hunter: build request: %v", err)}
	}
	req.Header.Set(headerAPIKey, key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hunter: %s %s: %v", method, path, err)}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hunter: read response: %v", err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{msg: fmt.Sprintf("hunter API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))}
		if resp.StatusCode == http.StatusUnauthorized {
			// 401 stays an *apiError (exit 1) while also carrying the
			// credential-rejection classification through the wrapper.
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

// apiMessage extracts Hunter's errors[0] details/id from an error body, falling
// back to the raw body when the envelope is missing or malformed.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			ID      string          `json:"id"`
			Details string          `json:"details"`
			Code    json.RawMessage `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && len(e.Errors) > 0 {
		first := e.Errors[0]
		switch {
		case first.Details != "" && first.ID != "":
			return first.ID + ": " + first.Details
		case first.Details != "":
			return first.Details
		case first.ID != "":
			return first.ID
		}
	}
	return string(body)
}

// decodeJSONObjectFlag validates a raw-JSON object flag value and returns the
// decoded map for merging into a request body.
func decodeJSONObjectFlag(name, raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("hunter: --%s is not a valid JSON object: %w", name, err)
	}
	return v, nil
}

// setIf writes key=value into q only when value is non-empty.
func setIf(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// setBodyIf writes key=value into body only when value is non-empty.
func setBodyIf(body map[string]any, key, value string) {
	if value != "" {
		body[key] = value
	}
}

// bindStringFlags declares a group of string flags in one pass, keeping the
// resource command constructors flat.
func bindStringFlags(cmd *cobra.Command, specs []stringFlag) {
	for _, sp := range specs {
		cmd.Flags().StringVar(sp.target, sp.name, "", sp.usage)
	}
}

// stringFlag pairs a --name/usage with its destination pointer.
type stringFlag struct {
	target *string
	name   string
	usage  string
}
