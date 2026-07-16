package microsoftonedrive

import (
	"context"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/msgraph"
)

// graph builds the shared Microsoft Graph request core for this Service. All
// Graph plumbing (retry, error/scope-hint formatting, credential
// classification, JSON emit) lives in the msgraph package so a fix lands once
// across all three Microsoft tools; only onedrive's raw content download and
// upload-session chunking stay tool-specific (see transfer.go).
func (s *Service) graph() *msgraph.Client {
	return &msgraph.Client{
		Provider:    "microsoft-onedrive",
		APILabel:    "microsoft-onedrive API error",
		ScopeHint:   scopeHint,
		ResolveBase: s.base,
		ResolveHTTP: s.client,
		ResolveOut:  s.stdout,
		Sleep:       s.sleep,
	}
}

// call performs one Graph API request with Bearer auth against a base-relative
// path, marshalling payload as JSON.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	return s.graph().DoJSON(ctx, token, method, path, query, payload, nil)
}

// callEndpoint performs one Graph API request against an absolute endpoint.
// path is used only for error context (it also carries the Graph
// @odata.nextLink verbatim when paging). A non-empty payload is sent with
// contentType.
func (s *Service) callEndpoint(ctx context.Context, token, method, path, endpoint, contentType string, payload []byte) ([]byte, error) {
	return s.graph().Do(ctx, token, msgraph.Request{
		Method:      method,
		Path:        path,
		Endpoint:    endpoint,
		ContentType: contentType,
		Body:        payload,
	})
}

// getContent GETs a base-relative path expecting raw bytes (e.g. an item's
// /content download, which Graph serves via a 302 the HTTP client follows). An
// empty body is a legitimate empty file, so the empty-GET guard is skipped.
func (s *Service) getContent(ctx context.Context, token, path string) ([]byte, error) {
	return s.graph().Do(ctx, token, msgraph.Request{
		Method:   http.MethodGet,
		Path:     path,
		Endpoint: s.base() + path,
		Raw:      true,
	})
}

// apiError builds the surfaced error for a non-2xx response (used by the
// upload-session chunk loop, which bypasses the shared request path).
func (s *Service) apiError(status int, path string, body []byte) error {
	return s.graph().APIError(status, path, body)
}

func (s *Service) emit(body []byte) error { return s.graph().Emit(body) }

func (s *Service) emitJSON(value any) error { return s.graph().EmitJSON(value) }
