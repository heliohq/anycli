package mercury

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newAccountListCmd lists the organization's accounts (GET /accounts). Mercury
// paginates accounts with cursor params (start_after / end_before), not offset.
func (s *Service) newAccountListCmd(token string) *cobra.Command {
	var limit int
	var order, startAfter, endBefore string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List accounts (GET /accounts)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if order != "" {
				q.Set("order", order)
			}
			if startAfter != "" {
				q.Set("start_after", startAfter)
			}
			if endBefore != "" {
				q.Set("end_before", endBefore)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/accounts", q)
			if err != nil {
				return err
			}
			return s.emitList(body, "accounts", "page")
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max accounts per page (1-1000; Mercury defaults to 1000)")
	cmd.Flags().StringVar(&order, "order", "", "sort order: asc|desc (Mercury defaults to asc)")
	cmd.Flags().StringVar(&startAfter, "start-after", "", "cursor: return accounts after this account id (forward paging)")
	cmd.Flags().StringVar(&endBefore, "end-before", "", "cursor: return accounts before this account id (reverse paging)")
	return cmd
}

// newAccountGetCmd fetches one account (GET /account/{id}).
func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <account-id>",
		Short:       "Get one account by id (GET /account/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/account/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emitObject(body)
		},
	}
	return cmd
}
