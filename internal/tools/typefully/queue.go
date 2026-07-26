package typefully

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newQueueCmd groups the posting-queue views and the recurring slot schedule.
func (s *Service) newQueueCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "queue", Short: "Inspect the posting queue and recurring slot schedule"}
	cmd.AddCommand(
		s.newQueueViewCmd(token),
		s.newQueueScheduleGetCmd(token),
		s.newQueueScheduleSetCmd(token),
	)
	return cmd
}

func (s *Service) newQueueViewCmd(token string) *cobra.Command {
	var socialSet, startDate, endDate string
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Show slots + scheduled drafts in a window (GET /v2/social-sets/{id}/queue)",
		Long: "--start-date and --end-date bound the window and Typefully rejects a span\n" +
			"longer than 62 days; omitting both lets it pick. The response pairs the\n" +
			"recurring slots with whatever scheduled draft occupies each, which is how\n" +
			"to see what `--publish-at next-free-slot` will actually pick without\n" +
			"guessing.",
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
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/queue"), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&startDate, "start-date", "", "window start (ISO-8601 date/datetime); window must be <= 62 days")
	cmd.Flags().StringVar(&endDate, "end-date", "", "window end (ISO-8601 date/datetime)")
	return cmd
}

func (s *Service) newQueueScheduleGetCmd(token string) *cobra.Command {
	var socialSet string
	cmd := &cobra.Command{
		Use:   "schedule-get",
		Short: "Get the recurring slot schedule (GET /v2/social-sets/{id}/queue/schedule)",
		Long: "The recurring rule — the weekly times `--publish-at next-free-slot` draws\n" +
			"from — not the occupancy. Which of those slots are already taken over a\n" +
			"date range is `queue view`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/queue/schedule"), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	return cmd
}

func (s *Service) newQueueScheduleSetCmd(token string) *cobra.Command {
	var socialSet, data string
	cmd := &cobra.Command{
		Use:   "schedule-set",
		Short: "Replace the recurring slot schedule (PUT /v2/social-sets/{id}/queue/schedule; needs ADMIN)",
		Long: "A PUT, not a merge: the --data body REPLACES the entire recurring schedule,\n" +
			"so any slot missing from it is removed. Start from what `queue\n" +
			"schedule-get` returns and edit that. This is the only command needing\n" +
			"ADMIN on the social set, so it can fail with a 403 where every other write\n" +
			"in this tool succeeds.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			decoded, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, scopedPath(socialSet, "/queue/schedule"), nil, decoded)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&data, "data", "", "raw JSON schedule body (slot rules); required")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}
