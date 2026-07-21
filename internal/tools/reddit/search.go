package reddit

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newSearchCmd is `reddit search`: link search, optionally restricted to one
// subreddit. Reddit search covers posts only (no comment search) and has no
// date-range filter; the tool exposes exactly what the API supports.
func (s *Service) newSearchCmd(token string) *cobra.Command {
	var query, subreddit, sort, timeRange, after string
	var limit int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search posts (optionally within one subreddit)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" {
				return &usageError{msg: "--query is required"}
			}
			if err := requireEnum("sort", sort, "relevance", "hot", "top", "new", "comments"); err != nil {
				return err
			}
			if err := requireEnum("time", timeRange, "hour", "day", "week", "month", "year", "all"); err != nil {
				return err
			}
			if err := requireLimit(limit); err != nil {
				return err
			}
			q := url.Values{"q": {query}, "type": {"link"}}
			if sort != "" {
				q.Set("sort", sort)
			}
			if timeRange != "" {
				q.Set("t", timeRange)
			}
			if limit != 0 {
				q.Set("limit", intToStr(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			path := "/search"
			if subreddit != "" {
				q.Set("restrict_sr", "1")
				path = "/r/" + url.PathEscape(subreddit) + "/search"
			}
			body, err := s.get(cmd.Context(), token, path, q)
			if err != nil {
				return err
			}
			return s.emitPostListing(jsonFlag(jsonMode(cmd)), body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query (required)")
	cmd.Flags().StringVar(&subreddit, "subreddit", "", "restrict the search to this subreddit")
	cmd.Flags().StringVar(&sort, "sort", "", "relevance|hot|top|new|comments")
	cmd.Flags().StringVar(&timeRange, "time", "", "hour|day|week|month|year|all")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum posts in this page (1-100)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page")
	return cmd
}
