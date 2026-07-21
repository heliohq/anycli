package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newCanvasCmd builds the `canvas` resource group: list / details / series /
// summary (GET export) plus trigger (POST act).
func (s *Service) newCanvasCmd(c *client) *cobra.Command {
	group := newGroupCmd("canvas", "Canvas inventory, analytics, and API-triggered sends")
	group.AddCommand(
		s.newCanvasListCmd(c),
		s.newCanvasDetailsCmd(c),
		s.newCanvasSeriesCmd(c),
		s.newCanvasSummaryCmd(c),
		s.newCanvasTriggerCmd(c),
	)
	return group
}

// newCanvasListCmd is `canvas list` (GET /canvas/list).
func (s *Service) newCanvasListCmd(c *client) *cobra.Command {
	var page int
	var includeArchived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Canvases (id + name), paginated",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().IntVar(&page, "page", 0, "0-indexed page of Canvases to return")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "include archived Canvases")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if cmd.Flags().Changed("page") {
			q.Set("page", strconv.Itoa(page))
		}
		if includeArchived {
			q.Set("include_archived", "true")
		}
		body, err := c.get(cmd.Context(), "/canvas/list", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newCanvasDetailsCmd is `canvas details` (GET /canvas/details).
func (s *Service) newCanvasDetailsCmd(c *client) *cobra.Command {
	var canvasID string
	cmd := &cobra.Command{
		Use:   "details",
		Short: "Get a Canvas's configuration and metadata",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&canvasID, "canvas-id", "", "Canvas API identifier (required)")
	_ = cmd.MarkFlagRequired("canvas-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("canvas_id", canvasID)
		body, err := c.get(cmd.Context(), "/canvas/details", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newCanvasSeriesCmd is `canvas series` (GET /canvas/data_series): step-level
// analytics over a window.
func (s *Service) newCanvasSeriesCmd(c *client) *cobra.Command {
	var canvasID, endingAt, startingAt string
	var length int
	cmd := &cobra.Command{
		Use:   "series",
		Short: "Get a Canvas's analytics time-series",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&canvasID, "canvas-id", "", "Canvas API identifier (required)")
	cmd.Flags().IntVar(&length, "length", 7, "number of days (max 14) ending at --ending-at")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (required)")
	cmd.Flags().StringVar(&startingAt, "starting-at", "", "ISO-8601 start date/time (optional)")
	_ = cmd.MarkFlagRequired("canvas-id")
	_ = cmd.MarkFlagRequired("ending-at")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("canvas_id", canvasID)
		q.Set("ending_at", endingAt)
		// Braze requires either length or starting_at alongside ending_at, and
		// rejects sending both. Prefer an explicit starting_at; otherwise apply
		// length (defaulting to 7) so a bare call always satisfies the contract.
		if startingAt != "" {
			q.Set("starting_at", startingAt)
		} else {
			q.Set("length", strconv.Itoa(length))
		}
		body, err := c.get(cmd.Context(), "/canvas/data_series", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newCanvasSummaryCmd is `canvas summary` (GET /canvas/data_summary): rollup
// analytics for a Canvas.
func (s *Service) newCanvasSummaryCmd(c *client) *cobra.Command {
	var canvasID, endingAt, startingAt string
	var length int
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Get rollup analytics for a Canvas",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&canvasID, "canvas-id", "", "Canvas API identifier (required)")
	cmd.Flags().IntVar(&length, "length", 7, "number of days (max 14) ending at --ending-at")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (required)")
	cmd.Flags().StringVar(&startingAt, "starting-at", "", "ISO-8601 start date/time (optional)")
	_ = cmd.MarkFlagRequired("canvas-id")
	_ = cmd.MarkFlagRequired("ending-at")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("canvas_id", canvasID)
		q.Set("ending_at", endingAt)
		// Braze requires either length or starting_at alongside ending_at, and
		// rejects sending both. Prefer an explicit starting_at; otherwise apply
		// length (defaulting to 7) so a bare call always satisfies the contract.
		if startingAt != "" {
			q.Set("starting_at", startingAt)
		} else {
			q.Set("length", strconv.Itoa(length))
		}
		body, err := c.get(cmd.Context(), "/canvas/data_summary", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newCanvasTriggerCmd is `canvas trigger` (POST /canvas/trigger/send): the
// recipients / canvas_entry_properties body is passed through --body; the tool
// only sets canvas_id. Permission-gated.
func (s *Service) newCanvasTriggerCmd(c *client) *cobra.Command {
	var canvasID, bodyFlag string
	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Send an API-triggered Canvas (permission-gated)",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&canvasID, "canvas-id", "", "Canvas API identifier (required)")
	cmd.Flags().StringVar(&bodyFlag, "body", "", "raw JSON object: recipients, canvas_entry_properties, broadcast, audience, …")
	_ = cmd.MarkFlagRequired("canvas-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := objectBodyFlag("body", bodyFlag, map[string]any{"canvas_id": canvasID})
		if err != nil {
			return err
		}
		body, err := c.post(cmd.Context(), "/canvas/trigger/send", payload)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
