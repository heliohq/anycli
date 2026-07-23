package sendgrid

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newSuppressionCmd(token string, region *string) *cobra.Command {
	cmd := &cobra.Command{Use: "suppression", Short: "Suppression lists (bounces, unsubscribes, blocks)"}
	cmd.AddCommand(
		s.newSuppressionListCmd(token, region, "bounces", "/suppression/bounces", "List bounced addresses (GET /v3/suppression/bounces)"),
		s.newSuppressionListCmd(token, region, "unsubscribes", "/suppression/unsubscribes", "List global unsubscribes (GET /v3/suppression/unsubscribes)"),
		s.newSuppressionListCmd(token, region, "blocks", "/suppression/blocks", "List blocked addresses (GET /v3/suppression/blocks)"),
	)
	return cmd
}

// newSuppressionListCmd builds one suppression subcommand. The three lists share
// the same paged GET shape (limit/offset), differing only in path.
func (s *Service) newSuppressionListCmd(token string, region *string, use, path, short string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("limit", intToString(limit))
			q.Set("offset", intToString(offset))
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 100, "max entries to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}
