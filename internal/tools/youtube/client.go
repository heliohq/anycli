package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, or a bad enum value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a YouTube non-2xx response or a transport
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

// call performs one YouTube Data API request with Bearer auth. A non-2xx
// surfaces the body's error message as an apiError carrying the HTTP status;
// 401/403 additionally carry the missing-scope reconnect hint and (for genuine
// auth failures) the credential-rejection classification. A transport failure
// is an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := s.base() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("youtube: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		hint := ""
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			hint = scopeHint
		}
		raw := fmt.Errorf("youtube API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), hint)
		classified := classifyCredentialError(resp.StatusCode, body, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// emit writes a provider JSON response to stdout. It refuses to write bytes
// that are not strictly valid JSON so --json output is always parseable.
func (s *Service) emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return &apiError{msg: "youtube: provider returned invalid JSON"}
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("youtube: encode output: %v", err), err: err}
	}
	_, err = s.stdout().Write(append(body, '\n'))
	return err
}

// emitOK writes the standard empty-body-mutation envelope {"ok":true,"id":…}.
// The id is omitted when empty (e.g. a bulk operation without a single id).
func (s *Service) emitOK(id string) error {
	out := map[string]any{"ok": true}
	if id != "" {
		out["id"] = id
	}
	return s.emitJSON(out)
}

// listResponse is the shared shape of every YouTube *ListResponse: an items
// array plus the paging cursor. The kind/etag/pageInfo envelope is dropped;
// items are kept verbatim so every requested `part` survives.
type listResponse struct {
	Items         []json.RawMessage `json:"items"`
	NextPageToken string            `json:"nextPageToken"`
}

// decodeList decodes a raw ListResponse body.
func decodeList(body []byte) (listResponse, error) {
	var lr listResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return listResponse{}, &apiError{msg: fmt.Sprintf("youtube: decode list response: %v", err), err: err}
	}
	return lr, nil
}

// emitList normalizes a ListResponse to {"items":[…],"nextPageToken":"…"},
// stripping the kind/etag/pageInfo envelope. nextPageToken is omitted when
// absent so callers see a clean end-of-list signal.
func (s *Service) emitList(lr listResponse) error {
	out := map[string]any{"items": itemsOrEmpty(lr.Items)}
	if lr.NextPageToken != "" {
		out["nextPageToken"] = lr.NextPageToken
	}
	return s.emitJSON(out)
}

// itemsOrEmpty guarantees a JSON array (never null) for the items field.
func itemsOrEmpty(items []json.RawMessage) []json.RawMessage {
	if items == nil {
		return []json.RawMessage{}
	}
	return items
}

// apiMessage extracts Google's error message from an error body, falling back
// to the raw body. YouTube shares the standard Google JSON error envelope.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Errors  []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if e.Error.Message != "" || e.Error.Status != "" {
			msg := e.Error.Message
			if len(e.Error.Errors) > 0 && e.Error.Errors[0].Reason != "" {
				msg = fmt.Sprintf("%s (%s)", msg, e.Error.Errors[0].Reason)
			}
			if e.Error.Status != "" && msg != "" {
				return fmt.Sprintf("%s: %s", e.Error.Status, msg)
			}
			if msg != "" {
				return msg
			}
			return e.Error.Status
		}
	}
	return string(body)
}
