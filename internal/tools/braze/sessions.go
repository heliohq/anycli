package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newSessionsCmd builds the `sessions` resource group: app-session time-series
// (GET export).
func (s *Service) newSessionsCmd(c *client) *cobra.Command {
	group := newGroupCmd("sessions", "App-session analytics")
	group.AddCommand(s.newSessionsSeriesCmd(c))
	return group
}

// newSessionsSeriesCmd is `sessions series` (GET /sessions/data_series):
// app-session counts over a window.
func (s *Service) newSessionsSeriesCmd(c *client) *cobra.Command {
	var endingAt, unit, appID, segmentID string
	var length int
	cmd := &cobra.Command{
		Use:   "series",
		Short: "Get app-session counts over time",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().IntVar(&length, "length", 7, "number of units (max 100) ending at --ending-at")
	cmd.Flags().StringVar(&unit, "unit", "", "time unit: day|hour (optional; default day)")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (optional; default now)")
	cmd.Flags().StringVar(&appID, "app-id", "", "restrict to a single app (optional)")
	cmd.Flags().StringVar(&segmentID, "segment-id", "", "restrict to a single segment (optional)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("length", strconv.Itoa(length))
		if unit != "" {
			q.Set("unit", unit)
		}
		if endingAt != "" {
			q.Set("ending_at", endingAt)
		}
		if appID != "" {
			q.Set("app_id", appID)
		}
		if segmentID != "" {
			q.Set("segment_id", segmentID)
		}
		body, err := c.get(cmd.Context(), "/sessions/data_series", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
