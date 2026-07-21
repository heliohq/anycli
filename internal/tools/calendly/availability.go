package calendly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAvailabilitySlotsCmd wraps GET /event_type_available_times. The API
// reference caps the [start_time, end_time] span at ~1 week and requires it to
// be in the future; the tool passes the range through unmodified and lets the
// live API validate — it never rejects a range client-side.
func (s *Service) newAvailabilitySlotsCmd(token string) *cobra.Command {
	var eventType, from, to string
	cmd := &cobra.Command{
		Use:   "slots",
		Short: "Open slots for an event type (GET /event_type_available_times)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("event_type", s.normalizeURI("event_types", eventType))
			q.Set("start_time", from)
			q.Set("end_time", to)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/event_type_available_times", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&eventType, "event-type", "", "event type URI or bare UUID")
	cmd.Flags().StringVar(&from, "from", "", "start_time (ISO-8601, future; span capped at ~1 week by the API)")
	cmd.Flags().StringVar(&to, "to", "", "end_time (ISO-8601)")
	_ = cmd.MarkFlagRequired("event-type")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// newAvailabilityBusyCmd wraps GET /user_busy_times (span ≤ 7 days per the API).
func (s *Service) newAvailabilityBusyCmd(token string) *cobra.Command {
	var user, from, to string
	cmd := &cobra.Command{
		Use:   "busy",
		Short: "Calendar busy view for a user (GET /user_busy_times, ≤7 days)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			userURI, err := s.resolveUserURI(cmd.Context(), token, user)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("user", userURI)
			q.Set("start_time", from)
			q.Set("end_time", to)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/user_busy_times", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&user, "user", "me", "user URI, bare UUID, or \"me\"")
	cmd.Flags().StringVar(&from, "from", "", "start_time (ISO-8601)")
	cmd.Flags().StringVar(&to, "to", "", "end_time (ISO-8601, ≤7 days after start)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// newAvailabilityScheduleCmd is the `schedule` subgroup; `list` wraps
// GET /user_availability_schedules.
func (s *Service) newAvailabilityScheduleCmd(token string) *cobra.Command {
	group := newGroupCmd("schedule", "Working-hours availability schedules")
	var user string
	list := &cobra.Command{
		Use:   "list",
		Short: "Working-hours schedules + date overrides (GET /user_availability_schedules)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			userURI, err := s.resolveUserURI(cmd.Context(), token, user)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("user", userURI)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/user_availability_schedules", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	list.Flags().StringVar(&user, "user", "me", "user URI, bare UUID, or \"me\"")
	group.AddCommand(list)
	return group
}
