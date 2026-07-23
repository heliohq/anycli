package mercury

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newRecipientListCmd lists the organization's payment recipients
// (GET /recipients). Cursor-paginated (start_after / end_before).
func (s *Service) newRecipientListCmd(token string) *cobra.Command {
	var limit int
	var startAfter, endBefore string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List payment recipients (GET /recipients)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if startAfter != "" {
				q.Set("start_after", startAfter)
			}
			if endBefore != "" {
				q.Set("end_before", endBefore)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/recipients", q)
			if err != nil {
				return err
			}
			return s.emitList(body, "recipients", "total", "page")
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max recipients per page")
	cmd.Flags().StringVar(&startAfter, "start-after", "", "cursor: return recipients after this recipient id (forward paging)")
	cmd.Flags().StringVar(&endBefore, "end-before", "", "cursor: return recipients before this recipient id (reverse paging)")
	return cmd
}

// newRecipientGetCmd fetches one recipient (GET /recipient/{id}).
func (s *Service) newRecipientGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <recipient-id>",
		Short:       "Get one recipient by id (GET /recipient/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/recipient/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emitObject(body)
		},
	}
	return cmd
}
