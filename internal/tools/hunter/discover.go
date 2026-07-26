package hunter

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newDiscoverCmd wraps POST /discover: company search by natural-language
// --query and/or a raw-JSON --filters object merged into the request body.
// Premium structured filters are plan-gated; this passes them through and
// surfaces Hunter's own error rather than pre-validating the caller's plan.
func (s *Service) newDiscoverCmd(key string) *cobra.Command {
	var query, filters string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Search for companies (POST /discover)",
		Long: "Finds COMPANIES, not people — the way in when no target domain exists\n" +
			"yet. `--query` is natural language (\"SaaS companies in France with\n" +
			"50-100 employees\"); `--filters` is a raw JSON object merged into the\n" +
			"request body verbatim, so any structured filter Hunter documents can be\n" +
			"passed through, and the two combine. Which premium filters actually\n" +
			"resolve depends on the plan — nothing is validated locally, so an\n" +
			"unsupported filter surfaces as Hunter's own error. Page with `--limit`\n" +
			"and `--offset`, then feed each result's domain to `domain-search`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if filters != "" {
				merged, err := decodeJSONObjectFlag("filters", filters)
				if err != nil {
					return err
				}
				for k, v := range merged {
					body[k] = v
				}
			}
			setBodyIf(body, "query", query)
			if cmd.Flags().Changed("limit") {
				body["limit"] = limit
			}
			if cmd.Flags().Changed("offset") {
				body["offset"] = offset
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/discover", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "natural-language company query")
	cmd.Flags().StringVar(&filters, "filters", "", "raw JSON object of structured filters (merged into the body)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}
