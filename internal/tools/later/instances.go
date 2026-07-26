package later

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newInstancesCmd lists the reporting instances (Later Influence workspaces)
// the credential can see: GET /v2/instances. The returned instanceIds seed the
// --instance-ids filter on `later campaigns`.
func (s *Service) newInstancesCmd(client *reportingClient) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "instances",
		Short: "List reporting instances the credential can access (GET /v2/instances)",
		Long: "The starting point when a report has to be scoped: the ids returned here\n" +
			"are what `campaigns --instance-ids` accepts, and they are also the only\n" +
			"way to see how many workspaces the credential reaches. `--limit` is 1-100;\n" +
			"omitting it takes the provider's own page size.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			if limit > 0 {
				query.Set("limit", strconv.Itoa(limit))
			}
			body, err := client.get(cmd.Context(), "/v2/instances", query)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max instances per page (1-100; omitted = provider default)")
	return cmd
}
