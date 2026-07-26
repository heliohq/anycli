package mailchimp

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSegmentCmd builds the segment group: list and members.
func (s *Service) newSegmentCmd(r *requester) *cobra.Command {
	group := newGroupCmd("segment", "Manage audience segments")
	group.AddCommand(
		s.newSegmentListCmd(r),
		s.newSegmentMembersCmd(r),
	)
	return group
}

func (s *Service) newSegmentListCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <list_id>",
		Short: "List segments (GET /lists/{list_id}/segments)",
		Long: "Segments are the audience's saved filters, and the `id` returned here is what\n" +
			"`campaign create --segment` targets. The rows distinguish a static segment (a\n" +
			"frozen set of members) from a saved conditional one that re-evaluates, and\n" +
			"each carries the `member_count` a campaign sent to it would currently reach.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := r.do(cmd.Context(), http.MethodGet, "/lists/"+url.PathEscape(args[0])+"/segments", listQuery(cmd), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd)
	return cmd
}

func (s *Service) newSegmentMembersCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members <list_id> <segment_id>",
		Short: "List members of a segment (GET /lists/{list_id}/segments/{segment_id}/members)",
		Long: "Takes both ids positionally, audience first and segment second. A conditional\n" +
			"segment is evaluated when the call runs, so two calls can differ with no\n" +
			"member data having changed in between; a static segment is stable. Returns\n" +
			"full member records, the same shape `member list` returns.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/lists/" + url.PathEscape(args[0]) + "/segments/" + url.PathEscape(args[1]) + "/members"
			body, err := r.do(cmd.Context(), http.MethodGet, path, listQuery(cmd), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd)
	return cmd
}
