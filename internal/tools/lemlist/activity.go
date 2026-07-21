package lemlist

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newActivityCmd exposes the event stream (opens, clicks, replies, bounces).
// GET /activities requires version=v2, which the service always sends.
func (s *Service) newActivityCmd(key string) *cobra.Command {
	cmd := newGroupCmd("activity", "Read the event stream (opens, clicks, replies, bounces)")
	cmd.AddCommand(s.newActivityListCmd(key))
	return cmd
}

func (s *Service) newActivityListCmd(key string) *cobra.Command {
	var activityType, campaignID, leadID, minDate, maxDate string
	var offset, limit int
	var isFirst bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List activities (GET /activities)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("version", "v2") // required by the endpoint
			if activityType != "" {
				q.Set("type", activityType)
			}
			if campaignID != "" {
				q.Set("campaignId", campaignID)
			}
			if leadID != "" {
				q.Set("leadId", leadID)
			}
			if minDate != "" {
				q.Set("minDate", minDate)
			}
			if maxDate != "" {
				q.Set("maxDate", maxDate)
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if isFirst {
				q.Set("isFirst", "true")
			}
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/activities", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&activityType, "type", "", "filter by activity type (e.g. emailsOpened, emailsReplied)")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "filter by campaign id")
	cmd.Flags().StringVar(&leadID, "lead-id", "", "filter by lead id")
	cmd.Flags().StringVar(&minDate, "min-date", "", "createdAt >= minDate (Unix seconds or ISO 8601)")
	cmd.Flags().StringVar(&maxDate, "max-date", "", "createdAt <= maxDate (Unix seconds or ISO 8601)")
	cmd.Flags().IntVar(&offset, "offset", 0, "records to skip; increment by limit to page")
	cmd.Flags().IntVar(&limit, "limit", 0, "max activities to return (max 100)")
	cmd.Flags().BoolVar(&isFirst, "is-first", false, "only the first activity per lead")
	return cmd
}
