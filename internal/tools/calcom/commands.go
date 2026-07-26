package calcom

import (
	"encoding/json"
	"net/url"

	"github.com/spf13/cobra"
)

// bookingStatuses is the closed set accepted by `booking list --status`; an
// out-of-set value is a usage error (exit 2), never forwarded to Cal.com.
var bookingStatuses = map[string]bool{"upcoming": true, "past": true, "cancelled": true}

// --- event-type ---

func (s *Service) newEventTypeListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the authenticated user's bookable event types",
		Long: "The entry point for everything else: it returns each type's numeric `id`\n" +
			"(what `--event-type-id` takes), its slug, and its length in minutes. That\n" +
			"length is what makes a slot's end time predictable — slot listings give\n" +
			"starts. Takes no filters or paging flags.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := s.getJSON(cmd.Context(), token, "/event-types", verEventTypes, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
}

func (s *Service) newEventTypeGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one event type by id",
		Long: "--id is required and is the numeric id from `event-type list`, not a slug.\n" +
			"It returns the type's full configuration — duration, locations, booking\n" +
			"questions, minimum notice — which is what explains why `slot list` returns\n" +
			"nothing for a range that looks free.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			data, err := s.getJSON(cmd.Context(), token, "/event-types/"+url.PathEscape(id), verEventTypes, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "event type id")
	return cmd
}

// --- slot ---

func (s *Service) newSlotListCmd(token string) *cobra.Command {
	var eventTypeID, start, end string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available slots for an event type within a time range",
		Long: "--event-type-id, --start and --end are all required, and start/end are\n" +
			"ISO-8601 instants in UTC. Keep the window to a few days: the range is\n" +
			"scanned against the user's schedules and busy calendars, and an open-ended\n" +
			"span is a slow, large response. An empty result is a real answer — it\n" +
			"usually means the schedule, a minimum-notice rule or an existing booking\n" +
			"blocks that window, not that the query was wrong.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if eventTypeID == "" || start == "" || end == "" {
				return &usageError{msg: "--event-type-id, --start and --end are required"}
			}
			q := url.Values{}
			q.Set("eventTypeId", eventTypeID)
			q.Set("start", start)
			q.Set("end", end)
			data, err := s.getJSON(cmd.Context(), token, "/slots", verSlots, q)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
	cmd.Flags().StringVar(&eventTypeID, "event-type-id", "", "event type id")
	cmd.Flags().StringVar(&start, "start", "", "range start (ISO 8601, UTC)")
	cmd.Flags().StringVar(&end, "end", "", "range end (ISO 8601, UTC)")
	return cmd
}

// --- booking ---

func (s *Service) newBookingListCmd(token string) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bookings (optionally filtered by status)",
		Long: "--status is validated locally against exactly `upcoming`, `past` and\n" +
			"`cancelled`; anything else is a usage error and is never sent to Cal.com.\n" +
			"With no --status every booking comes back, cancelled ones included, so a\n" +
			"\"what is on the calendar\" question must filter. Each row carries the `uid`\n" +
			"that every other booking command needs.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if status != "" {
				if !bookingStatuses[status] {
					return &usageError{msg: "--status must be one of upcoming|past|cancelled"}
				}
				q.Set("status", status)
			}
			data, err := s.getJSON(cmd.Context(), token, "/bookings", verBookings, q)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter: upcoming|past|cancelled")
	return cmd
}

func (s *Service) newBookingGetCmd(token string) *cobra.Command {
	var uid string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one booking by uid",
		Long: "--uid is required and is the booking's string uid from `booking list`, NOT\n" +
			"the numeric event-type id. It returns the attendees, the resolved meeting\n" +
			"location or video link, and the current status — read it before cancelling\n" +
			"or rescheduling so the attendee being disturbed is the expected one.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if uid == "" {
				return &usageError{msg: "--uid is required"}
			}
			data, err := s.getJSON(cmd.Context(), token, "/bookings/"+url.PathEscape(uid), verBookings, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
	cmd.Flags().StringVar(&uid, "uid", "", "booking uid")
	return cmd
}

func (s *Service) newBookingCreateCmd(token string) *cobra.Command {
	var eventTypeID int
	var start, name, email, tz, notes, metadata string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Book a meeting on the user's behalf",
		Long: "--event-type-id, --start, --attendee-name, --attendee-email and\n" +
			"--attendee-tz are all required. --start is an ISO-8601 instant sent in\n" +
			"UTC, while --attendee-tz is the attendee's IANA zone (`America/New_York`)\n" +
			"used for their own notifications — the two are independent and both are\n" +
			"needed. Confirm the start against `slot list` first: booking a time that\n" +
			"is not open is rejected by Cal.com, and a booked one sends a real calendar\n" +
			"invitation immediately. --notes fills the event type's default notes field\n" +
			"and --metadata takes a JSON object.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if eventTypeID == 0 || start == "" || name == "" || email == "" || tz == "" {
				return &usageError{msg: "--event-type-id, --start, --attendee-name, --attendee-email and --attendee-tz are required"}
			}
			body := map[string]any{
				"eventTypeId": eventTypeID,
				"start":       start,
				"attendee": map[string]any{
					"name":     name,
					"email":    email,
					"timeZone": tz,
				},
			}
			if notes != "" {
				// The default "notes" booking field lives under bookingFieldsResponses;
				// only sent when provided so the common path stays minimal.
				body["bookingFieldsResponses"] = map[string]any{"notes": notes}
			}
			if metadata != "" {
				m, err := parseJSONObject(metadata)
				if err != nil {
					return &usageError{msg: "--metadata must be a JSON object: " + err.Error()}
				}
				body["metadata"] = m
			}
			data, err := s.postJSON(cmd.Context(), token, "/bookings", verBookings, body)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
	cmd.Flags().IntVar(&eventTypeID, "event-type-id", 0, "event type id")
	cmd.Flags().StringVar(&start, "start", "", "start time (ISO 8601, UTC)")
	cmd.Flags().StringVar(&name, "attendee-name", "", "attendee name")
	cmd.Flags().StringVar(&email, "attendee-email", "", "attendee email")
	cmd.Flags().StringVar(&tz, "attendee-tz", "", "attendee IANA time zone (e.g. America/New_York)")
	cmd.Flags().StringVar(&notes, "notes", "", "optional additional notes")
	cmd.Flags().StringVar(&metadata, "metadata", "", "optional metadata JSON object")
	return cmd
}

func (s *Service) newBookingCancelCmd(token string) *cobra.Command {
	var uid, reason string
	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel a booking",
		Long: "--uid is required — the booking's string uid, not the event-type id.\n" +
			"Cancelling notifies the attendee straight away and cannot be undone:\n" +
			"putting the meeting back means creating a new booking, which is a fresh\n" +
			"invitation. --reason is optional and is surfaced to the attendee in that\n" +
			"notification.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if uid == "" {
				return &usageError{msg: "--uid is required"}
			}
			body := map[string]any{}
			if reason != "" {
				body["cancellationReason"] = reason
			}
			data, err := s.postJSON(cmd.Context(), token, "/bookings/"+url.PathEscape(uid)+"/cancel", verBookings, body)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
	cmd.Flags().StringVar(&uid, "uid", "", "booking uid")
	cmd.Flags().StringVar(&reason, "reason", "", "optional cancellation reason")
	return cmd
}

func (s *Service) newBookingRescheduleCmd(token string) *cobra.Command {
	var uid, start, reason string
	cmd := &cobra.Command{
		Use:   "reschedule",
		Short: "Move a booking to a new start time",
		Long: "--uid and --start are both required; --start is an ISO-8601 instant in\n" +
			"UTC. Rescheduling keeps the same event type and attendee and re-notifies\n" +
			"them, so prefer it over cancel-then-create, which drops the thread. Check\n" +
			"the new time against `slot list` first. Cal.com may issue a new uid for\n" +
			"the rescheduled booking, so take the uid for any follow-up from this\n" +
			"command's response rather than reusing the old one.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if uid == "" || start == "" {
				return &usageError{msg: "--uid and --start are required"}
			}
			body := map[string]any{"start": start}
			if reason != "" {
				body["rescheduleReason"] = reason
			}
			data, err := s.postJSON(cmd.Context(), token, "/bookings/"+url.PathEscape(uid)+"/reschedule", verBookings, body)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
	cmd.Flags().StringVar(&uid, "uid", "", "booking uid")
	cmd.Flags().StringVar(&start, "start", "", "new start time (ISO 8601, UTC)")
	cmd.Flags().StringVar(&reason, "reason", "", "optional reschedule reason")
	return cmd
}

// --- schedule ---

func (s *Service) newScheduleListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the user's availability schedules",
		Long: "Returns the working-hours schedules that decide which times `slot list`\n" +
			"can ever offer — days, hours and the schedule's own time zone. Read it\n" +
			"when open time is unexpectedly scarce: the schedule is the constraint, and\n" +
			"it is read-only through this tool.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := s.getJSON(cmd.Context(), token, "/schedules", verSchedules, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
}

// --- me ---

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the authenticated Cal.com profile",
		Long: "Identifies which Cal.com user the connected credential belongs to,\n" +
			"including their default time zone — worth reading before interpreting\n" +
			"anything time-related, since bookings are created against this user's\n" +
			"calendar. It takes no arguments and makes the cheapest connectivity check\n" +
			"available here.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := s.getJSON(cmd.Context(), token, "/me", verMe, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(data)
		},
	}
}

// parseJSONObject parses a JSON object flag value, rejecting non-object JSON.
func parseJSONObject(raw string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return m, nil
}
