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
		Long: "The defaults hide most of the calendar: with no flags it returns CONFIRMED\n" +
			"and UPCOMING events only, so cancellations need `--state canceled|all` and\n" +
			"history needs `--period past|all`. `--limit` is 20, capped at 100, and the\n" +
			"`metadata` cursors continue it via `--after`. Only bookings made through\n" +
			"SavvyCal appear — meetings created directly in the underlying calendar do\n" +
			"not.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes the event id from `event list` and returns the whole booking:\n" +
			"scheduler, times, state, booking-form answers and conferencing details.\n" +
			"Worth calling again right after `event create` — conferencing information\n" +
			"such as a Zoom URL can attach a moment later and be missing from the\n" +
			"create response.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "Books a real meeting on the account's calendar and notifies the other\n" +
			"party. `--start` and `--end` must match an available slot EXACTLY: call\n" +
			"`link slots <link_id>` first and echo a slot's `start_at` / `end_at`, since\n" +
			"a time that merely looks free is refused rather than adjusted.\n" +
			"`--time-zone` is the SCHEDULER's IANA zone, not the account's, and decides\n" +
			"what their invitation displays. `--field id=value` repeats to answer the\n" +
			"link's booking-form questions — the ids come from `link get` — and\n" +
			"`--metadata` passes a raw JSON object through untouched. A rejected booking\n" +
			"returns 422 with the offending field named.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "Cancels the booking and SavvyCal notifies the other party; `--reason` is\n" +
			"optional and is shown to them, so leaving it off cancels without\n" +
			"explanation. There is no un-cancel — restoring the meeting means a fresh\n" +
			"`event create` against a currently available slot. The cancelled event\n" +
			"stays readable through `event list --state canceled`.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
