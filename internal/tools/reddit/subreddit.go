package reddit

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// newSubredditCmd groups subreddit reads: about, rules, posts.
func (s *Service) newSubredditCmd(token string) *cobra.Command {
	cmd := newGroup("subreddit", "Read subreddit metadata and posts")
	cmd.AddCommand(
		s.newSubredditAboutCmd(token),
		s.newSubredditRulesCmd(token),
		s.newSubredditPostsCmd(token),
	)
	return cmd
}

func (s *Service) newSubredditAboutCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "about <name>",
		Short:       "Subreddit metadata (subscribers, description)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.get(cmd.Context(), token, "/r/"+url.PathEscape(args[0])+"/about", nil)
			if err != nil {
				return err
			}
			var t thing
			if err := json.Unmarshal(body, &t); err != nil {
				return &apiError{msg: fmt.Sprintf("reddit: decode subreddit: %v", err), err: err}
			}
			d, err := decodeThingData(t.Data)
			if err != nil {
				return err
			}
			if jsonMode(cmd) {
				return s.emitValue(toSubreddit(d))
			}
			return s.emitLine(fmt.Sprintf("r/%s\t%d subscribers\t%s", d.DisplayName, d.Subscribers, d.PublicDesc))
		},
	}
}

func (s *Service) newSubredditRulesCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "rules <name>",
		Short:       "Subreddit posting rules",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.get(cmd.Context(), token, "/r/"+url.PathEscape(args[0])+"/about/rules", nil)
			if err != nil {
				return err
			}
			var rules struct {
				Rules []struct {
					ShortName   string `json:"short_name"`
					Description string `json:"description"`
					Kind        string `json:"kind"`
				} `json:"rules"`
			}
			if err := json.Unmarshal(body, &rules); err != nil {
				return &apiError{msg: fmt.Sprintf("reddit: decode rules: %v", err), err: err}
			}
			if jsonMode(cmd) {
				for _, r := range rules.Rules {
					if err := s.emitValue(map[string]any{
						"short_name":  r.ShortName,
						"description": r.Description,
						"kind":        r.Kind,
					}); err != nil {
						return err
					}
				}
				return nil
			}
			for i, r := range rules.Rules {
				if err := s.emitLine(fmt.Sprintf("%d. %s", i+1, r.ShortName)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func (s *Service) newSubredditPostsCmd(token string) *cobra.Command {
	var sort, timeRange, after string
	var limit int
	cmd := &cobra.Command{
		Use:         "posts <name>",
		Short:       "List posts in a subreddit",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireEnum("sort", sort, "hot", "new", "top", "rising"); err != nil {
				return err
			}
			if err := requireEnum("time", timeRange, "hour", "day", "week", "month", "year", "all"); err != nil {
				return err
			}
			if err := requireLimit(limit); err != nil {
				return err
			}
			if sort == "" {
				sort = "hot"
			}
			q := url.Values{}
			if timeRange != "" {
				q.Set("t", timeRange)
			}
			if limit != 0 {
				q.Set("limit", intToStr(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			body, err := s.get(cmd.Context(), token, "/r/"+url.PathEscape(args[0])+"/"+sort, q)
			if err != nil {
				return err
			}
			return s.emitPostListing(jsonFlag(jsonMode(cmd)), body)
		},
	}
	cmd.Flags().StringVar(&sort, "sort", "", "hot|new|top|rising (default hot)")
	cmd.Flags().StringVar(&timeRange, "time", "", "hour|day|week|month|year|all (top/controversial only)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum posts in this page (1-100)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page")
	return cmd
}
