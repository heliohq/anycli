package sendgrid

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newListCmd(token string, region *string) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "Marketing lists (ls)"}
	cmd.AddCommand(s.newListLsCmd(token, region))
	return cmd
}

func (s *Service) newListLsCmd(token string, region *string) *cobra.Command {
	var pageSize int
	cmd := &cobra.Command{
		Use:         "ls",
		Short:       "List marketing lists (GET /v3/marketing/lists)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("page_size", intToString(pageSize))
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/marketing/lists", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "lists per page (1-1000)")
	return cmd
}
