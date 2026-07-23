package typefully

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAnalyticsCmd groups the basic post/follower metrics (X only today).
func (s *Service) newAnalyticsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "analytics", Short: "Basic post/follower analytics (X only)"}
	cmd.AddCommand(s.newAnalyticsPostsCmd(token), s.newAnalyticsFollowersCmd(token))
	return cmd
}

func (s *Service) newAnalyticsPostsCmd(token string) *cobra.Command {
	var socialSet, platform, startDate, endDate string
	var includeReplies bool
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "posts",
		Short:       "Per-post metrics (GET /v2/social-sets/{id}/analytics/{platform}/posts)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if startDate != "" {
				q.Set("start_date", startDate)
			}
			if endDate != "" {
				q.Set("end_date", endDate)
			}
			if includeReplies {
				q.Set("include_replies", "true")
			}
			addPaging(q, limit, offset)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/analytics/"+platform+"/posts"), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&platform, "platform", "x", "analytics platform (currently x)")
	cmd.Flags().StringVar(&startDate, "start-date", "", "window start (ISO-8601)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "window end (ISO-8601)")
	cmd.Flags().BoolVar(&includeReplies, "include-replies", false, "include reply posts")
	registerPaging(cmd, &limit, &offset)
	return cmd
}

func (s *Service) newAnalyticsFollowersCmd(token string) *cobra.Command {
	var socialSet, platform, startDate, endDate string
	cmd := &cobra.Command{
		Use:         "followers",
		Short:       "Follower metrics over time (GET /v2/social-sets/{id}/analytics/{platform}/followers)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if startDate != "" {
				q.Set("start_date", startDate)
			}
			if endDate != "" {
				q.Set("end_date", endDate)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/analytics/"+platform+"/followers"), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&platform, "platform", "x", "analytics platform (currently x)")
	cmd.Flags().StringVar(&startDate, "start-date", "", "window start (ISO-8601)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "window end (ISO-8601)")
	return cmd
}
