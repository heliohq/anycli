package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newEventsCmd builds the `events` resource group: list (custom-event name
// discovery) and series (per-event occurrence analytics), both GET export.
func (s *Service) newEventsCmd(c *client) *cobra.Command {
	group := newGroupCmd("events", "Custom-event discovery and occurrence analytics")
	group.AddCommand(
		s.newEventsListCmd(c),
		s.newEventsSeriesCmd(c),
	)
	return group
}

// newEventsListCmd is `events list` (GET /events/list): the custom-event names
// registered in the workspace — the discovery primitive before series.
func (s *Service) newEventsListCmd(c *client) *cobra.Command {
	var page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List custom-event names, paginated",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().IntVar(&page, "page", 0, "0-indexed page of event names to return")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if cmd.Flags().Changed("page") {
			q.Set("page", strconv.Itoa(page))
		}
		body, err := c.get(cmd.Context(), "/events/list", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newEventsSeriesCmd is `events series` (GET /events/data_series): occurrences
// of one custom event over a window.
func (s *Service) newEventsSeriesCmd(c *client) *cobra.Command {
	var event, unit, endingAt string
	var length int
	cmd := &cobra.Command{
		Use:         "series",
		Short:       "Get occurrences of a custom event over time",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&event, "event", "", "custom-event name (required)")
	cmd.Flags().IntVar(&length, "length", 7, "number of units (max 100) ending at --ending-at")
	cmd.Flags().StringVar(&unit, "unit", "", "time unit: day|hour (optional; default day)")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (optional; default now)")
	_ = cmd.MarkFlagRequired("event")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("event", event)
		q.Set("length", strconv.Itoa(length))
		if unit != "" {
			q.Set("unit", unit)
		}
		if endingAt != "" {
			q.Set("ending_at", endingAt)
		}
		body, err := c.get(cmd.Context(), "/events/data_series", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
