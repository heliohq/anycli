package reddit

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newSubsCmd is `reddit subs list`: the connected account's subscriptions.
func (s *Service) newSubsCmd(token string) *cobra.Command {
	cmd := newGroup("subs", "The connected account's subreddit subscriptions")
	cmd.AddCommand(s.newSubsListCmd(token))
	return cmd
}

func (s *Service) newSubsListCmd(token string) *cobra.Command {
	var after string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List subscribed subreddits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLimit(limit); err != nil {
				return err
			}
			q := url.Values{}
			if limit != 0 {
				q.Set("limit", intToStr(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			body, err := s.get(cmd.Context(), token, "/subreddits/mine/subscriber", q)
			if err != nil {
				return err
			}
			return s.emitSubredditListing(jsonFlag(jsonMode(cmd)), body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum subreddits in this page (1-100)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page")
	return cmd
}
