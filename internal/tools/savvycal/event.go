package savvycal

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newEventCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Events booked through SavvyCal (list, get, create, cancel)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		s.newEventListCmd(token),
		s.newEventGetCmd(token),
		s.newEventCreateCmd(token),
		s.newEventCancelCmd(token),
	)
	return cmd
}

func (s *Service) newEventListCmd(token string) *cobra.Command {
	var state, period, after, before string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events scheduled via SavvyCal (GET /v1/events)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if state != "" {
				q.Set("state", state)
			}
			if period != "" {
				q.Set("period", period)
			}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", itoa(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			if before != "" {
				q.Set("before", before)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/events", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&state, "state", "", "confirmed|canceled|all (default confirmed)")
	cmd.Flags().StringVar(&period, "period", "", "past|upcoming|all (default upcoming)")
	cmd.Flags().IntVar(&limit, "limit", 20, "page size (max 100)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}

func (s *Service) newEventGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <event_id>",
		Short: "Fetch a single event (GET /v1/events/:event_id)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/events/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newEventCreateCmd(token string) *cobra.Command {
	var displayName, email, start, end, timeZone, metadata string
	var fields []string
	cmd := &cobra.Command{
		Use:   "create <link_id>",
		Short: "Create an event on a scheduling link (POST /v1/links/:link_id/events)",
		Long: "Create an event on a scheduling link. start/end must match an " +
			"available slot — call `link slots <link_id>` first to find valid times.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"display_name": displayName,
				"email":        email,
				"start_at":     start,
				"end_at":       end,
				"time_zone":    timeZone,
			}
			parsed, err := parseFields(fields)
			if err != nil {
				return err
			}
			if len(parsed) > 0 {
				body["fields"] = parsed
			}
			if metadata != "" {
				v, err := decodeJSONFlag("metadata", metadata)
				if err != nil {
					return err
				}
				body["metadata"] = v
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/links/"+url.PathEscape(args[0])+"/events", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&displayName, "display-name", "", "scheduler's display name")
	cmd.Flags().StringVar(&email, "email", "", "scheduler's email")
	cmd.Flags().StringVar(&start, "start", "", "ISO-8601 start time (must match an available slot)")
	cmd.Flags().StringVar(&end, "end", "", "ISO-8601 end time (must match an available slot)")
	cmd.Flags().StringVar(&timeZone, "time-zone", "", "IANA time zone, e.g. America/New_York")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "booking form field as id=value (repeatable)")
	cmd.Flags().StringVar(&metadata, "metadata", "", "metadata JSON object (raw passthrough)")
	_ = cmd.MarkFlagRequired("display-name")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	_ = cmd.MarkFlagRequired("time-zone")
	return cmd
}

func (s *Service) newEventCancelCmd(token string) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "cancel <event_id>",
		Short: "Cancel an event (POST /v1/events/:event_id/cancel)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if reason != "" {
				body["cancel_reason"] = reason
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/events/"+url.PathEscape(args[0])+"/cancel", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "cancellation reason (optional)")
	return cmd
}
