package calendly

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newBookCreateCmd wraps the 2026 Scheduling API POST /invitees to book a slot
// directly on an invitee's behalf. Body: event_type (URI), UTC start_time,
// invitee {name, email, timezone}, optional location {kind, location}, optional
// guests (emails).
//
// This endpoint requires the connected Calendly account to be on a PAID plan;
// free-tier accounts get a 403, which the tool surfaces verbatim rather than
// hiding (no silent degradation).
func (s *Service) newBookCreateCmd(token string) *cobra.Command {
	var eventType, start, name, email, timezone, locationKind, location string
	var guests []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Book a slot on an invitee's behalf (POST /invitees, Scheduling API; requires a paid plan)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			invitee := map[string]any{
				"name":     name,
				"email":    email,
				"timezone": timezone,
			}
			body := map[string]any{
				"event_type": s.normalizeURI("event_types", eventType),
				"start_time": start,
				"invitee":    invitee,
			}
			if locationKind != "" {
				loc := map[string]any{"kind": locationKind}
				if location != "" {
					loc["location"] = location
				}
				body["location"] = loc
			}
			if len(guests) > 0 {
				body["guests"] = guests
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/invitees", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&eventType, "event-type", "", "event type URI or bare UUID to book")
	cmd.Flags().StringVar(&start, "start", "", "start_time in UTC (ISO-8601)")
	cmd.Flags().StringVar(&name, "name", "", "invitee full name")
	cmd.Flags().StringVar(&email, "email", "", "invitee email")
	cmd.Flags().StringVar(&timezone, "timezone", "", "invitee IANA timezone, e.g. America/New_York")
	cmd.Flags().StringVar(&locationKind, "location-kind", "", "location kind (per the event type's location rules)")
	cmd.Flags().StringVar(&location, "location", "", "location value (e.g. phone number or address)")
	cmd.Flags().StringArrayVar(&guests, "guest", nil, "additional guest email (repeatable)")
	_ = cmd.MarkFlagRequired("event-type")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("timezone")
	return cmd
}
