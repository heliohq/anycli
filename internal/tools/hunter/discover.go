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
		Args:  cobra.NoArgs,
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
