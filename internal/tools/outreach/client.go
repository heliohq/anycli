package outreach

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

// mediaType is the JSON:API 1.0 media type Outreach requires on both
// Content-Type (415 otherwise) and Accept.
const mediaType = "application/vnd.api+json"

const maxErrorBodyBytes = 8 << 10

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, or a bad value. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Outreach non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classification still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Outreach JSON:API request: Bearer auth plus the JSON:API
// media type on Content-Type (when there is a body) and Accept. A non-2xx
// surfaces the JSON:API errors[] body as an apiError carrying the HTTP status; a
// transport failure surfaces as an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + encodeQuery(query)
	}

	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("outreach: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("outreach: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", mediaType)
	if payload != nil {
		req.Header.Set("Content-Type", mediaType)
	}

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("outreach: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("outreach: read response: %v", err), err: err}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, body, token)
	}
	return body, nil
}

// encodeQuery encodes url.Values while preserving the literal square brackets
// Outreach's JSON:API params rely on (filter[...], page[...], fields[...],
// actionParams[...]). url.Values.Encode() percent-encodes "[" and "]", which
// Outreach accepts, but keeping them literal keeps request URLs readable and
// matches the documented examples.
func encodeQuery(query url.Values) string {
	encoded := query.Encode()
	encoded = strings.ReplaceAll(encoded, "%5B", "[")
	encoded = strings.ReplaceAll(encoded, "%5D", "]")
	return encoded
}

// newAPIError builds an apiError from a non-2xx Outreach response, surfacing the
// JSON:API errors[] contents (id/title/detail(s)/source pointer) verbatim, and
// classifies credential-rejection (401 / 403 scope error) so the engine can
// invalidate the token.
func newAPIError(status int, body []byte, token string) error {
	message := outreachErrorMessage(body)
	if message == "" {
		message = "empty response body"
	}
	message = strings.ReplaceAll(message, token, "[REDACTED]")
	if len(message) > maxErrorBodyBytes {
		message = message[:maxErrorBodyBytes] + "…"
	}
	hint := statusHint(status)
	raw := fmt.Errorf("outreach API error (HTTP %d %s): %s%s", status, http.StatusText(status), message, hint)
	classified := classifyCredentialError(status, body, raw)
	return &apiError{msg: classified.Error(), status: status, err: classified}
}

// statusHint returns an actionable clause for the failures an agent most often
// hits.
func statusHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "; access token is invalid or expired — reconnect Outreach"
	case http.StatusForbidden:
		return "; the token is missing a required OAuth scope for this resource"
	case http.StatusTooManyRequests:
		return "; rate limit exceeded — retry after the reset window (see X-RateLimit-Reset / Retry-After)"
	}
	return ""
}

// jsonAPIError is one entry in a JSON:API errors[] array. Outreach documents
// each entry as carrying id, title, and detail(s), with a source.pointer for
// field-specific validation failures. "detail" is the JSON:API standard; the
// Outreach docs also show "details", so both are decoded.
type jsonAPIError struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Details string `json:"details"`
	Source  struct {
		Pointer   string `json:"pointer"`
		Parameter string `json:"parameter"`
	} `json:"source"`
}

// outreachErrorMessage renders the JSON:API errors[] array into a single line,
// preserving each entry's id/title/detail and source pointer. It falls back to
// the raw body when the payload is not the documented error shape.
func outreachErrorMessage(body []byte) string {
	var envelope struct {
		Errors []jsonAPIError `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Errors) == 0 {
		return strings.TrimSpace(string(body))
	}
	parts := make([]string, 0, len(envelope.Errors))
	for _, e := range envelope.Errors {
		var b strings.Builder
		if e.ID != "" {
			fmt.Fprintf(&b, "%s: ", e.ID)
		}
		b.WriteString(e.Title)
		detail := e.Detail
		if detail == "" {
			detail = e.Details
		}
		if detail != "" {
			if e.Title != "" {
				b.WriteString(" — ")
			}
			b.WriteString(detail)
		}
		if e.Source.Pointer != "" {
			fmt.Fprintf(&b, " (%s)", e.Source.Pointer)
		} else if e.Source.Parameter != "" {
			fmt.Fprintf(&b, " (%s)", e.Source.Parameter)
		}
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return strings.Join(parts, "; ")
}
