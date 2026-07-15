package notion

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// newSearchCmd is the top-level `search` (POST /v1/search): search pages and
// data sources inside the Notion workspace (never across external connectors —
// that is Notion-AI-only). Output is always JSON.
func (s *Service) newSearchCmd(token string) *cobra.Command {
	var query, typ string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search Notion pages and data sources",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&query, "query", "", "search query (required)")
	cmd.Flags().StringVar(&typ, "type", "", "object filter: page|data_source (omit to search both)")
	pf := registerPaginationFlags(cmd)
	_ = cmd.MarkFlagRequired("query")

	validateType := enumValidator("type", "page", "data_source")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if typ != "" {
			if err := validateType(typ); err != nil {
				return err
			}
		}
		fetch := func(ctx context.Context, cursor string) ([]byte, error) {
			payload := map[string]any{"query": query}
			if typ != "" {
				payload["filter"] = map[string]any{"property": "object", "value": typ}
			}
			if pf.pageSize > 0 {
				payload["page_size"] = pf.pageSize
			}
			if cursor != "" {
				payload["start_cursor"] = cursor
			}
			// The `data_source` object filter value belongs to the 2026-03-11
			// data model, so search runs on markdownVersion.
			return s.callWithVersion(ctx, token, http.MethodPost, "/search", payload, markdownVersion)
		}
		body, err := paginate(cmd.Context(), pf.all, pf.startCursor, fetch)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newFetchCmd is the top-level `fetch <id>`: resolve the id type, then read a
// page as markdown or a database / data source as JSON. --json forces JSON for
// a page too.
func (s *Service) newFetchCmd(token string) *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "fetch <id>",
		Short: "Fetch a page (markdown), or a database / data source (JSON)",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&typ, "type", "", "id type: page|database|data_source (skip type detection)")

	validateType := enumValidator("type", "page", "database", "data_source")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		raw := strings.TrimSpace(args[0])
		if strings.EqualFold(raw, "self") {
			return &usageError{msg: `fetch does not accept "self"; use "user get self" for the current user`}
		}
		if typ != "" {
			if err := validateType(typ); err != nil {
				return err
			}
		}
		id, err := resolveID(raw)
		if err != nil {
			return err
		}
		kind, err := s.resolveFetchType(cmd.Context(), token, raw, id, typ)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		switch kind {
		case "page":
			body, err := s.readPageMarkdown(cmd.Context(), token, id)
			if err != nil {
				return err
			}
			if jsonMode {
				return s.emitJSON(body)
			}
			return s.emitMarkdown(body)
		case "data_source":
			body, err := s.callWithVersion(cmd.Context(), token, http.MethodGet, "/data_sources/"+url.PathEscape(id), nil, markdownVersion)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		case "database":
			body, err := s.callWithVersion(cmd.Context(), token, http.MethodGet, "/databases/"+url.PathEscape(id), nil, markdownVersion)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		default:
			return &usageError{msg: "unresolved fetch type"}
		}
	}
	return cmd
}

// resolveFetchType decides the id kind by design-304 priority: a full URL is
// judged by shape first; an explicit --type wins next; a bare uuid with no
// --type falls through to the endpoint probe.
func (s *Service) resolveFetchType(ctx context.Context, token, raw, id, typ string) (string, error) {
	if isURL(raw) {
		if t := detectURLType(raw); t != "" {
			return t, nil
		}
	}
	if typ != "" {
		return typ, nil
	}
	kind, err := s.probeIDType(ctx, token, id)
	if errors.Is(err, errIndeterminateType) {
		return "", &usageError{msg: "cannot determine the type of " + id + "; pass --type page|database|data_source"}
	}
	return kind, err
}

// detectURLType judges a Notion URL by slug shape: a database URL carries a
// view id in the `v` query parameter; otherwise it is a page. It returns ""
// when the URL cannot be parsed (caller falls back to --type / probe).
func detectURLType(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Query().Get("v") != "" {
		return "database"
	}
	return "page"
}

// errIndeterminateType is returned by probeIDType when every endpoint probe
// reported a clean not-this-type / no-access miss (no hard failure). Callers
// wrap it with command-appropriate guidance, since only `fetch` exposes --type.
var errIndeterminateType = errors.New("indeterminate id type")

// probeIDType types a bare uuid by trying the endpoints in order: page markdown
// → data source → database. The first 2xx wins. A clean miss on every endpoint
// (404 / 403 / 400) yields errIndeterminateType for the caller to phrase. A
// hard failure — 401 / credential rejection, a 5xx, or a transport error — is
// returned immediately so exit-1 and the credential-rejection classification
// survive (design 227 OAuth refresh depends on it) instead of being masked as a
// "pass --type" usage error. Shared by fetch, --new-parent wire resolution, and
// the page move id-type check.
func (s *Service) probeIDType(ctx context.Context, token, id string) (string, error) {
	if _, err := s.readPageMarkdown(ctx, token, id); err == nil {
		return "page", nil
	} else if fatal := fatalProbeError(err); fatal != nil {
		return "", fatal
	}
	if _, err := s.callWithVersion(ctx, token, http.MethodGet, "/data_sources/"+url.PathEscape(id), nil, markdownVersion); err == nil {
		return "data_source", nil
	} else if fatal := fatalProbeError(err); fatal != nil {
		return "", fatal
	}
	if _, err := s.callWithVersion(ctx, token, http.MethodGet, "/databases/"+url.PathEscape(id), nil, markdownVersion); err == nil {
		return "database", nil
	} else if fatal := fatalProbeError(err); fatal != nil {
		return "", fatal
	}
	return "", errIndeterminateType
}

// fatalProbeError decides whether a probe error must abort the endpoint chain
// rather than fall through to the next type. A 404 / 403 / 400 means "not this
// type / not accessible via this endpoint" and the probe should continue
// (nil). A 401 (or any credential rejection), a 5xx, or a transport error
// (status 0) is fatal and is returned so the caller propagates exit-1 and the
// credential-rejection signal.
func fatalProbeError(err error) error {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		// Not an API error (should not happen on this path) — treat as fatal.
		return err
	}
	if execution.IsCredentialRejected(apiErr.err) {
		return err
	}
	switch apiErr.status {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound:
		return nil // clean mismatch — advance to the next endpoint
	}
	if apiErr.status == 0 || apiErr.status >= 500 {
		return err // transport failure or server error — fatal
	}
	return nil
}
