package fullstory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// apiError is a runtime/API failure (exit 1): a FullStory non-2xx, transport
// error, or decode failure. status is the HTTP status when known (0 otherwise);
// the wrapped cause preserves credential-rejection classification.
type apiError struct {
	status int
	msg    string
	cause  error
}

func (e *apiError) Error() string { return e.msg }

func (e *apiError) Unwrap() error { return e.cause }

// usageError is a param/usage failure (exit 2): a bad flag combination, missing
// required flag, or malformed --prop value detected before any HTTP call.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// call performs one FullStory API request with the raw-Basic auth header and
// returns the response body. A 401 marks the credential rejected; any other
// non-2xx becomes an apiError carrying FullStory's message. FullStory 429
// (monthly server-event quota exceeded) surfaces its reason verbatim.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("fullstory: encode request: %v", err)}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("fullstory: build request: %v", err)}
	}
	// FullStory's non-standard scheme: the raw key verbatim after "Basic ",
	// never base64(user:password) and never Bearer.
	req.Header.Set("Authorization", "Basic "+key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("fullstory: %s %s: %v", method, path, err), cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("fullstory: read response: %v", err), cause: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		aErr := &apiError{
			status: resp.StatusCode,
			msg:    fmt.Sprintf("fullstory API error (HTTP %d): %s", resp.StatusCode, apiMessage(body)),
		}
		if resp.StatusCode == http.StatusUnauthorized {
			aErr.cause = execution.RejectCredential(fmt.Errorf("%s", aErr.msg))
		}
		return nil, aErr
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

// apiMessage extracts FullStory's error message from an error body, falling
// back to the raw body. FullStory error bodies carry code/message.
func apiMessage(body []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details string `json:"details"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Message != "" || e.Details != "") {
		switch {
		case e.Message != "" && e.Details != "":
			return e.Message + ": " + e.Details
		case e.Message != "":
			return e.Message
		default:
			return e.Details
		}
	}
	return strings.TrimSpace(string(body))
}

// parseProps turns repeated --prop "key=value" flags into a properties map.
// Values are decoded as JSON scalars when they parse (numbers, booleans, null),
// so numeric/boolean custom properties keep their type; anything else is kept
// as a string. FullStory v2 infers property types, so this mirrors its model.
func parseProps(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(pairs))
	for _, pair := range pairs {
		key, raw, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, &usageError{msg: fmt.Sprintf("--prop %q must be key=value", pair)}
		}
		out[key] = scalarValue(raw)
	}
	return out, nil
}

// scalarValue coerces a raw flag string to a typed JSON scalar when it parses
// cleanly as an integer, float, or boolean; otherwise it stays a string.
func scalarValue(raw string) any {
	if raw == "true" || raw == "false" {
		return raw == "true"
	}
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	return raw
}
