package calendly

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

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, or an unresolvable input. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Calendly non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for *credentialRejectedError still resolves through it.
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

// call performs one Calendly API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx becomes
// an *apiError carrying Calendly's title/message. Transport failures become an
// *apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("calendly: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("calendly: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("calendly: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("calendly: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{
			msg:    fmt.Sprintf("calendly API error (HTTP %d): %s", resp.StatusCode, apiMessage(body)),
			status: resp.StatusCode,
		}
		if resp.StatusCode == http.StatusUnauthorized {
			apiErr.err = execution.RejectCredential(fmt.Errorf("%s", apiErr.msg))
			return nil, apiErr
		}
		return nil, apiErr
	}
	return body, nil
}

// apiMessage extracts Calendly's error title/message from an error body,
// falling back to the raw body. Calendly errors carry {title, message,
// details}.
func apiMessage(body []byte) string {
	var e struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Title != "" || e.Message != "") {
		switch {
		case e.Title != "" && e.Message != "":
			return e.Title + ": " + e.Message
		case e.Title != "":
			return e.Title
		default:
			return e.Message
		}
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// normalizeURI expands a bare UUID into the canonical Calendly resource URI for
// the given collection (e.g. "users", "event_types", "scheduled_events"). A
// value that already looks like a URI (contains "://") is returned unchanged, so
// callers can pass either form. Empty stays empty.
func (s *Service) normalizeURI(collection, value string) string {
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		return value
	}
	return s.baseURL() + "/" + collection + "/" + value
}

// uuidOf returns the trailing path segment of a resource URI, or the value
// unchanged when it is already a bare UUID. Used for the path-parameter
// endpoints (event-type get, event get/invitees/cancel) which take a {uuid} in
// the path rather than a URI query param.
func uuidOf(value string) string {
	value = strings.TrimRight(value, "/")
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}

// meUser is the subset of GET /users/me the tool needs to bootstrap URIs.
type meUser struct {
	Resource struct {
		URI                 string `json:"uri"`
		CurrentOrganization string `json:"current_organization"`
	} `json:"resource"`
}

// resolveMe fetches GET /users/me once and returns the authenticated user's own
// URI and current organization URI. Callers use it to expand the literal "me"
// user token and the --org organization scope.
func (s *Service) resolveMe(ctx context.Context, token string) (userURI, orgURI string, err error) {
	body, err := s.call(ctx, token, http.MethodGet, "/users/me", nil, nil)
	if err != nil {
		return "", "", err
	}
	var m meUser
	if err := json.Unmarshal(body, &m); err != nil {
		return "", "", &apiError{msg: fmt.Sprintf("calendly: decode /users/me: %v", err), err: err}
	}
	if m.Resource.URI == "" {
		return "", "", &apiError{msg: "calendly: /users/me returned no resource.uri"}
	}
	return m.Resource.URI, m.Resource.CurrentOrganization, nil
}

// resolveUserURI expands a --user flag value into a full user URI. The literal
// "me" (or an empty value) triggers a cached GET /users/me; anything else is
// normalized (bare UUID → users/{uuid} URI, full URI passthrough).
func (s *Service) resolveUserURI(ctx context.Context, token, value string) (string, error) {
	if value == "" || value == "me" {
		userURI, _, err := s.resolveMe(ctx, token)
		return userURI, err
	}
	return s.normalizeURI("users", value), nil
}

// addPaging applies the shared --count / --page-token cursor-pagination flags to
// a query value set. count <= 0 is omitted (provider default applies).
func addPaging(q url.Values, count int, pageToken string) {
	if count > 0 {
		q.Set("count", strconv.Itoa(count))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
}
