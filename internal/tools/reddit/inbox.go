package reddit

import (
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newInboxCmd groups inbox reads and mark-read.
func (s *Service) newInboxCmd(token string) *cobra.Command {
	cmd := newGroup("inbox", "Read the inbox and mark items read")
	cmd.AddCommand(
		s.newInboxListCmd(token),
		s.newInboxMarkReadCmd(token),
	)
	return cmd
}

func (s *Service) newInboxListCmd(token string) *cobra.Command {
	var filter, after string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List inbox items (replies, mentions, private messages)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireEnum("filter", filter, "all", "unread", "mentions"); err != nil {
				return err
			}
			if err := requireLimit(limit); err != nil {
				return err
			}
			segment := "inbox"
			switch filter {
			case "unread":
				segment = "unread"
			case "mentions":
				segment = "mentions"
			}
			q := url.Values{}
			if limit != 0 {
				q.Set("limit", intToStr(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			body, err := s.get(cmd.Context(), token, "/message/"+segment, q)
			if err != nil {
				return err
			}
			return s.emitCommentListing(jsonFlag(jsonMode(cmd)), body)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "all|unread|mentions (default all)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum items in this page (1-100)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page")
	return cmd
}

func (s *Service) newInboxMarkReadCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "mark-read <fullname>...",
		Short: "Mark one or more inbox items read (t4_/t1_ fullnames)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := requireFullname(id); err != nil {
					return err
				}
			}
			form := url.Values{"id": {strings.Join(args, ",")}}
			if _, err := s.postForm(cmd.Context(), token, "/api/read_message", form); err != nil {
				return err
			}
			if jsonMode(cmd) {
				return s.emitValue(map[string]any{"marked_read": args})
			}
			return s.emitLine("marked read: " + strings.Join(args, " "))
		},
	}
}
