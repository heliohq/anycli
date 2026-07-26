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
		Long: "The starting point for almost everything else: each audience's `id` is the\n" +
			"`<list_id>` that the member, segment and `campaign create` calls demand. Most\n" +
			"accounts hold only a handful, so an unpaged call is usually the whole set.\n" +
			"Each row otherwise carries full contact, permission-reminder and statistics\n" +
			"blocks, so `--fields lists.id,lists.name` cuts the response down sharply.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "Returns one audience with its `stats` block — `member_count`,\n" +
			"`unsubscribe_count`, `open_rate`, `click_rate`, `last_campaign_id` — which\n" +
			"sizes an audience in one call instead of paging `member list`. Takes a list\n" +
			"id, never an audience name. Only `--fields` applies; the paging flags are not\n" +
			"registered here.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
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
