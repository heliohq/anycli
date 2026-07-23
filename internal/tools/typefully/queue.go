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
		Use:         "view",
		Short:       "Show slots + scheduled drafts in a window (GET /v2/social-sets/{id}/queue)",
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
		Use:         "schedule-get",
		Short:       "Get the recurring slot schedule (GET /v2/social-sets/{id}/queue/schedule)",
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
		Use:         "schedule-set",
		Short:       "Replace the recurring slot schedule (PUT /v2/social-sets/{id}/queue/schedule; needs ADMIN)",
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
