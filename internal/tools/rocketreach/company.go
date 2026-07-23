package rocketreach

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCompanyLookupCmd is `company lookup` (GET /api/v2/company/lookup):
// firmographics for one company by name or domain.
func (s *Service) newCompanyLookupCmd(key string) *cobra.Command {
	var name, domain string
	cmd := &cobra.Command{
		Use:         "lookup",
		Short:       "Look up a company's firmographics (GET /company/lookup)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			switch {
			case domain != "":
				q.Set("domain", domain)
			case name != "":
				q.Set("name", name)
			default:
				return &usageError{msg: "provide --domain or --name"}
			}
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/api/v2/company/lookup", q, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "company name")
	cmd.Flags().StringVar(&domain, "domain", "", "company domain, e.g. acme.com")
	return cmd
}

// newCompanySearchCmd is `company search` (POST /api/v2/company/search): find
// companies by firmographic filters. Common filters are repeatable-value flags;
// --json-query is the escape hatch for the full RocketReach query object.
func (s *Service) newCompanySearchCmd(key string) *cobra.Command {
	var name, domain, jsonQuery string
	var pageSize, start int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search companies by firmographics (POST /company/search)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := map[string]any{}
			if jsonQuery != "" {
				decoded, err := decodeJSONQuery(jsonQuery)
				if err != nil {
					return err
				}
				query = decoded
			}
			addStringFilter(query, "name", name)
			addStringFilter(query, "domain", domain)
			if len(query) == 0 {
				return &usageError{msg: "provide at least one filter (--name, --domain, or --json-query)"}
			}
			payload := map[string]any{"query": query}
			if pageSize > 0 {
				payload["page_size"] = pageSize
			}
			if start > 0 {
				payload["start"] = start
			}
			body, err := s.call(cmd.Context(), key, http.MethodPost, "/api/v2/company/search", nil, payload)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "match on company name")
	cmd.Flags().StringVar(&domain, "domain", "", "match on company domain")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "max results per page")
	cmd.Flags().IntVar(&start, "start", 0, "1-based result offset for pagination")
	cmd.Flags().StringVar(&jsonQuery, "json-query", "", "raw RocketReach query object as JSON (escape hatch)")
	return cmd
}
