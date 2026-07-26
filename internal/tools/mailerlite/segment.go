package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSegmentCmd builds the `mailerlite segment` command tree. Segments are
// rule-defined and read-only via the API: list them and read their members.
func (s *Service) newSegmentCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "segment", Short: "Segments (list, subscribers) — read-only"}
	cmd.AddCommand(
		s.newSegmentListCmd(token),
		s.newSegmentSubscribersCmd(token),
	)
	return cmd
}

func (s *Service) newSegmentListCmd(token string) *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List segments (GET /segments)",
		Long: "Page-numbered with --page. Segments are rule-defined audiences maintained\n" +
			"in MailerLite's UI and are entirely read-only over this API: there is no\n" +
			"create, update or delete, so a new audience that has to be built from here\n" +
			"must be a group instead.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/segments", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}

func (s *Service) newSegmentSubscribersCmd(token string) *cobra.Command {
	var status, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "subscribers <id>",
		Short: "List a segment's subscribers (GET /segments/{id}/subscribers)",
		Long: "Cursor-paged with --cursor from `meta.next_cursor`; --status narrows by\n" +
			"subscriber status. Membership is computed from the segment's rules, so\n" +
			"nobody can be added or removed here — changing who is in a segment means\n" +
			"changing its rules in the UI, or switching to a group.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("filter[status]", status)
			}
			setLimitCursor(cmd, q, limit, cursor)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/segments/"+url.PathEscape(args[0])+"/subscribers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active|unsubscribed|unconfirmed|bounced|junk")
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	return cmd
}
