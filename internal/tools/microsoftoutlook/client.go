package microsoftoutlook

import (
	"context"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/msgraph"
)

// graph builds the shared Microsoft Graph request core for this Service. All
// Graph plumbing (retry, error/scope-hint formatting, credential
// classification, JSON emit) lives in the msgraph package so a fix lands once
// across all three Microsoft tools; only outlook's extra-headers path stays
// here.
func (s *Service) graph() *msgraph.Client {
	return &msgraph.Client{
		Provider:    "microsoft-outlook",
		APILabel:    "microsoft-outlook API error",
		ScopeHint:   scopeHint,
		ResolveBase: s.base,
		ResolveHTTP: s.client,
		ResolveOut:  s.stdout,
		Sleep:       s.sleep,
	}
}

// call performs one Graph request with Bearer auth and no extra headers.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	return s.callH(ctx, token, method, path, query, payload, nil)
}

// callH performs one Graph request with Bearer auth plus per-request extra
// headers (outlook's raw-MIME message fetch needs a Prefer header).
func (s *Service) callH(ctx context.Context, token, method, path string, query url.Values, payload any, headers map[string]string) ([]byte, error) {
	return s.graph().DoJSON(ctx, token, method, path, query, payload, headers)
}

func (s *Service) emit(body []byte) error { return s.graph().Emit(body) }

func (s *Service) emitJSON(value any) error { return s.graph().EmitJSON(value) }
