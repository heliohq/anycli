package surveymonkey

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxErrorBodyBytes = 8 << 10

// usageError is a parameter / usage error: a missing required flag or a bad flag
// value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a SurveyMonkey non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// get performs one authenticated GET against a v3-relative path (leading slash
// included, e.g. "/surveys") with optional query params, returning the raw
// response body on 2xx or an apiError otherwise.
func (s *Service) get(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("surveymonkey: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("surveymonkey: GET %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("surveymonkey: read response: %v", err), err: err}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, path, body, token)
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (provider-neutral,
// agent-consumable per built-in service conventions).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// smErrorBody is the SurveyMonkey error envelope: { "error": { id, name,
// message, http_status_code } }. id is the SurveyMonkey error code as a string
// (e.g. "1014").
type smErrorBody struct {
	Error struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"error"`
}

// isAnswerEndpoint reports whether path is one of the two answer-reading
// endpoints (response bulk -> /responses/bulk, response get ->
// /responses/{id}/details). Survey structure (/surveys/{id}/details) also ends
// in /details but does not sit under /responses, so it is excluded.
func isAnswerEndpoint(path string) bool {
	if !strings.Contains(path, "/responses") {
		return false
	}
	return strings.HasSuffix(path, "/bulk") || strings.HasSuffix(path, "/details")
}

// newAPIError builds an apiError from a non-2xx SurveyMonkey response, mapping
// the provider's permission/plan/region/rate-limit codes to actionable
// messages so none fall through to an opaque 403. It classifies genuine
// credential rejection (401 / auth codes) via classifyCredentialError.
func newAPIError(status int, path string, body []byte, token string) error {
	code, message := parseSMError(body)

	raw := message
	if raw == "" {
		raw = strings.TrimSpace(string(body))
	}
	raw = strings.ReplaceAll(raw, token, "[REDACTED]")
	if len(raw) > maxErrorBodyBytes {
		raw = raw[:maxErrorBodyBytes] + "…"
	}

	hint := errorHint(status, code, path)
	label := code
	if label == "" {
		label = http.StatusText(status)
	}
	apiErr := fmt.Errorf("surveymonkey API error (HTTP %d, code %s): %s%s", status, label, raw, hint)
	classified := classifyCredentialError(status, code, apiErr)
	return &apiError{
		msg:    classified.Error(),
		status: status,
		err:    classified,
	}
}

// errorHint returns an actionable clause for the permission/plan/region/rate
// codes an agent most often hits, empty otherwise.
func errorHint(status int, code, path string) string {
	switch code {
	case "1014":
		// Scope-not-granted. On the answer endpoints this is the free-plan
		// signal that the optional paid responses_read_detail scope is ungranted.
		if isAnswerEndpoint(path) {
			return "; reading survey answers requires the responses_read_detail permission, which needs a paid SurveyMonkey plan — reconnect after upgrading"
		}
		return "; the connected account has not granted the permission this request needs"
	case "1015":
		return "; the connected SurveyMonkey account's plan does not permit this request — a paid SurveyMonkey plan is required"
	case "1018":
		return "; this SurveyMonkey account is served from a datacenter/region not supported by this integration"
	case "1017", "1040":
		return "; rate limit reached — retry after the per-minute/daily reset window"
	}
	if status == http.StatusTooManyRequests {
		return "; rate limit reached — retry after the per-minute/daily reset window"
	}
	return ""
}

// parseSMError extracts the SurveyMonkey error code (id) and message from an
// error body, tolerating a non-JSON body.
func parseSMError(body []byte) (code, message string) {
	var e smErrorBody
	if err := json.Unmarshal(body, &e); err != nil {
		return "", ""
	}
	return e.Error.ID, e.Error.Message
}
