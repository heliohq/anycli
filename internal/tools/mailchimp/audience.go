package mailchimp

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAudienceCmd builds the audience (list) group: list and get.
func (s *Service) newAudienceCmd(r *requester) *cobra.Command {
	group := newGroupCmd("audience", "Manage audiences (lists)")
	group.AddCommand(
		s.newAudienceListCmd(r),
		s.newAudienceGetCmd(r),
	)
	return group
}

func (s *Service) newAudienceListCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audiences (GET /lists)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := r.do(cmd.Context(), http.MethodGet, "/lists", listQuery(cmd), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd)
	return cmd
}

func (s *Service) newAudienceGetCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <list_id>",
		Short: "Get one audience (GET /lists/{list_id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if f, _ := cmd.Flags().GetString("fields"); f != "" {
				q.Set("fields", f)
			}
			body, err := r.do(cmd.Context(), http.MethodGet, "/lists/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().String("fields", "", "comma-separated fields projection (passthrough)")
	return cmd
}
