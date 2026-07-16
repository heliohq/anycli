package microsoftcalendar

import (
	"context"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/msgraph"
)

// graph builds the shared Microsoft Graph request core for this Service. All
// Graph plumbing (retry, error/scope-hint formatting, credential
// classification, JSON emit) lives in the msgraph package so a fix lands once
// across all three Microsoft tools; calendar only contributes its default
// Accept header.
func (s *Service) graph() *msgraph.Client {
	return &msgraph.Client{
		Provider:       "microsoft-calendar",
		APILabel:       "microsoft-calendar API error",
		ScopeHint:      scopeHint,
		ResolveBase:    s.base,
		ResolveHTTP:    s.client,
		ResolveOut:     s.stdout,
		Sleep:          s.sleep,
		DefaultHeaders: map[string]string{"Accept": "application/json"},
	}
}

// call performs one Graph API request with Bearer auth, marshalling payload as
// JSON. See msgraph.Client.Do for the retry/error contract.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	return s.graph().DoJSON(ctx, token, method, path, query, payload, nil)
}

func (s *Service) emit(body []byte) error { return s.graph().Emit(body) }

func (s *Service) emitJSON(value any) error { return s.graph().EmitJSON(value) }
