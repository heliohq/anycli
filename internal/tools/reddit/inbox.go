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
		Long: "`--filter` is `all` (default), `unread` or `mentions`, and each is a\n" +
			"different Reddit endpoint rather than a client-side filter. Items come\n" +
			"back in the comment shape, so a private message and a comment reply look\n" +
			"alike and are told apart by their fullname prefix — `t4_` versus `t1_`.\n" +
			"Reading does NOT mark anything read: `--filter unread` keeps returning\n" +
			"the same items until `inbox mark-read` is called. `--limit` is 1-100,\n" +
			"`--after` pages.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes one or more fullnames as positional arguments and marks them all in\n" +
			"a single request; each is validated as a fullname before anything is\n" +
			"sent, so one bad argument fails the whole call rather than half of it.\n" +
			"Since reading never marks anything, this is what stops\n" +
			"`inbox list --filter unread` from returning the same items forever.\n" +
			"There is no mark-unread counterpart.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: writeAction,
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
