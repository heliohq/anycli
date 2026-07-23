package instagram

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// mediaFields are the default fields for media list / get.
const mediaFields = "id,caption,media_type,media_url,permalink,timestamp,like_count,comments_count"

// mediaInsightMetrics are the default per-media insight metrics.
const mediaInsightMetrics = "reach,likes,comments,saved,shares"

func (s *Service) newMediaListCmd(token string) *cobra.Command {
	var (
		limit  int
		after  string
		fields string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List the account's media (GET /me/media)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("fields", firstNonEmpty(fields, mediaFields))
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			body, err := s.get(cmd.Context(), token, "/me/media", q)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max items per page")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (paging.cursors.after)")
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated field list (default: media summary)")
	return cmd
}

func (s *Service) newMediaGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:         "get <media_id>",
		Short:       "Get one media object (GET /{media_id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("fields", firstNonEmpty(fields, mediaFields))
			body, err := s.get(cmd.Context(), token, "/"+url.PathEscape(args[0]), q)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated field list (default: media summary)")
	return cmd
}

func (s *Service) newMediaInsightsCmd(token string) *cobra.Command {
	var metrics string
	cmd := &cobra.Command{
		Use:         "insights <media_id>",
		Short:       "Per-media insights (GET /{media_id}/insights)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("metric", firstNonEmpty(metrics, mediaInsightMetrics))
			body, err := s.get(cmd.Context(), token, "/"+url.PathEscape(args[0])+"/insights", q)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&metrics, "metrics", "", "comma-separated metrics (default: reach,likes,comments,saved,shares)")
	return cmd
}
