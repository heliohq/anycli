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
		Use:         "list",
		Short:       "List the authenticated user's bookable event types",
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
		Use:         "get",
		Short:       "Get one event type by id",
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
		Use:         "list",
		Short:       "List available slots for an event type within a time range",
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
		Use:         "list",
		Short:       "List bookings (optionally filtered by status)",
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
		Use:         "get",
		Short:       "Get one booking by uid",
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
		Use:         "create",
		Short:       "Book a meeting on the user's behalf",
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
		Use:         "cancel",
		Short:       "Cancel a booking",
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
		Use:         "reschedule",
		Short:       "Move a booking to a new start time",
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
		Use:         "list",
		Short:       "List the user's availability schedules",
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
		Use:         "me",
		Short:       "Show the authenticated Cal.com profile",
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
