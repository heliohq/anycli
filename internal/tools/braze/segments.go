package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newSegmentsCmd builds the `segments` resource group: list / details / series
// (all GET export).
func (s *Service) newSegmentsCmd(c *client) *cobra.Command {
	group := newGroupCmd("segments", "Segment inventory and size analytics")
	group.AddCommand(
		s.newSegmentsListCmd(c),
		s.newSegmentsDetailsCmd(c),
		s.newSegmentsSeriesCmd(c),
	)
	return group
}

// newSegmentsListCmd is `segments list` (GET /segments/list).
func (s *Service) newSegmentsListCmd(c *client) *cobra.Command {
	var page int
	var sortDirection string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List segments (id + name), paginated",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().IntVar(&page, "page", 0, "0-indexed page of segments to return")
	cmd.Flags().StringVar(&sortDirection, "sort-direction", "", "asc|desc by creation time")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if cmd.Flags().Changed("page") {
			q.Set("page", strconv.Itoa(page))
		}
		if sortDirection != "" {
			q.Set("sort_direction", sortDirection)
		}
		body, err := c.get(cmd.Context(), "/segments/list", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newSegmentsDetailsCmd is `segments details` (GET /segments/details).
func (s *Service) newSegmentsDetailsCmd(c *client) *cobra.Command {
	var segmentID string
	cmd := &cobra.Command{
		Use:         "details",
		Short:       "Get a segment's configuration and metadata",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&segmentID, "segment-id", "", "segment API identifier (required)")
	_ = cmd.MarkFlagRequired("segment-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("segment_id", segmentID)
		body, err := c.get(cmd.Context(), "/segments/details", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newSegmentsSeriesCmd is `segments series` (GET /segments/data_series): the
// segment's estimated size over time.
func (s *Service) newSegmentsSeriesCmd(c *client) *cobra.Command {
	var segmentID, endingAt string
	var length int
	cmd := &cobra.Command{
		Use:         "series",
		Short:       "Get a segment's size over time",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&segmentID, "segment-id", "", "segment API identifier (required)")
	cmd.Flags().IntVar(&length, "length", 7, "number of days (max 100) ending at --ending-at")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (optional; default now)")
	_ = cmd.MarkFlagRequired("segment-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("segment_id", segmentID)
		q.Set("length", strconv.Itoa(length))
		if endingAt != "" {
			q.Set("ending_at", endingAt)
		}
		body, err := c.get(cmd.Context(), "/segments/data_series", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
