package stripe

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// searchResources maps the top-level `search --resource <name>` selector to the
// Stripe search endpoint base path. Only the resources Stripe exposes a
// /search endpoint for are listed (Stripe Search Query Language).
var searchResources = map[string]string{
	"customers":     "/customers",
	"charges":       "/charges",
	"invoices":      "/invoices",
	"subscriptions": "/subscriptions",
	"prices":        "/prices",
}

// newSearchCmd is the top-level Stripe Search Query Language entry point:
// `search --resource charges --query "amount>1000"`. It dispatches to the same
// per-resource /search endpoint the resource groups expose.
func (s *Service) newSearchCmd(token string) *cobra.Command {
	var resource, query, page string
	var limit int
	var params []string
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search a resource with Stripe Search Query Language",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			basePath, ok := searchResources[resource]
			if !ok {
				return &usageError{msg: fmt.Sprintf("stripe: --resource must be one of %s, got %q", searchResourceNames(), resource)}
			}
			q, err := searchQuery(query, limit, page, params)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, basePath+"/search", callOpts{query: q})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&resource, "resource", "", "resource to search: "+searchResourceNames())
	cmd.Flags().StringVar(&query, "query", "", "Stripe Search Query Language string (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size 1-100")
	cmd.Flags().StringVar(&page, "page", "", "pagination cursor from a previous search's next_page")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra query key=value (repeatable)")
	return cmd
}

// newResourceSearchCmd builds a `search --query` subcommand scoped to one
// resource's /search endpoint (used inside a resource group, e.g. customer).
func (s *Service) newResourceSearchCmd(token, basePath string) *cobra.Command {
	var query, page string
	var limit int
	var params []string
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search with Stripe Search Query Language",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := searchQuery(query, limit, page, params)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, basePath+"/search", callOpts{query: q})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Stripe Search Query Language string (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size 1-100")
	cmd.Flags().StringVar(&page, "page", "", "pagination cursor from a previous search's next_page")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra query key=value (repeatable)")
	return cmd
}

// searchQuery builds the query values for a /search request. query is required
// (Stripe rejects a search with no query, but we fail earlier with a clear
// usage error).
func searchQuery(query string, limit int, page string, params []string) (url.Values, error) {
	if strings.TrimSpace(query) == "" {
		return nil, &usageError{msg: "stripe: --query is required for search"}
	}
	q := url.Values{}
	q.Set("query", query)
	if limit != 0 {
		if limit < 1 || limit > 100 {
			return nil, &usageError{msg: fmt.Sprintf("stripe: --limit must be 1-100, got %d", limit)}
		}
		q.Set("limit", strconv.Itoa(limit))
	}
	if page != "" {
		q.Set("page", page)
	}
	if err := applyParams(q, params); err != nil {
		return nil, err
	}
	return q, nil
}

// searchResourceNames returns the sorted, comma-joined set of searchable
// resource selectors for help/error text.
func searchResourceNames() string {
	names := make([]string, 0, len(searchResources))
	for n := range searchResources {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// newGetCmd is the arbitrary read passthrough: `get <path>` performs a plain
// GET against api.stripe.com for any long-tail resource without a dedicated
// verb. The path may be given with or without the leading /v1 (a bare
// "charges" or "/charges" both resolve under the /v1 base). Repeatable --param
// entries become query params.
func (s *Service) newGetCmd(token string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         "get <path>",
		Short:       "Raw GET passthrough for any Stripe read (e.g. get account)",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := normalizeGetPath(args[0])
			q := url.Values{}
			if err := applyParams(q, params); err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, callOpts{query: q})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "query key=value (repeatable)")
	return cmd
}

// normalizeGetPath turns a caller path into a base-relative path under /v1. The
// service base URL already carries /v1, so a leading "/v1" or "v1" prefix is
// stripped and the remainder is rooted with a single leading slash.
func normalizeGetPath(raw string) string {
	p := strings.TrimSpace(raw)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "v1/")
	if p == "v1" {
		p = ""
	}
	return "/" + p
}
