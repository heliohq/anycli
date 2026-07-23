package freshservice

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newAssetCmd exposes the CMDB assets surface — the ITSM differentiator over a
// help-desk tool.
func (s *Service) newAssetCmd(c *client) *cobra.Command {
	cmd := newResourceGroup("asset", "CMDB assets: list, get")
	cmd.AddCommand(
		s.newAssetListCmd(c),
		s.newAssetGetCmd(c),
	)
	return cmd
}

// newAssetListCmd → GET /assets. --filter passes a query expression through to
// Freshservice's asset filter (e.g. name:'macbook').
func (s *Service) newAssetListCmd(c *client) *cobra.Command {
	var filter string
	var perPage, page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List assets (GET /assets)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePerPage(perPage); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("per_page", strconv.Itoa(perPage))
			if filter != "" {
				// Freshservice wraps the asset filter expression in quotes,
				// mirroring the ticket filter contract.
				q.Set("filter", `"`+filter+`"`)
			}
			return s.emitListResult(cmd, c, "/assets", "assets", q, page, perPage)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", `asset filter expression, e.g. name:'MacBook'`)
	cmd.Flags().IntVar(&perPage, "per-page", defaultPerPage, "results per page (max 100)")
	cmd.Flags().IntVar(&page, "page", 1, "1-based page number")
	return cmd
}

// newAssetGetCmd → GET /assets/{display_id}. Assets are addressed by their
// display id, not the internal id.
func (s *Service) newAssetGetCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <display-id>",
		Short:       "Get one asset by display id (GET /assets/{display_id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, _, err := c.call(cmd.Context(), http.MethodGet, "/assets/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitResource(body, "asset")
		},
	}
}
