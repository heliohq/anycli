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
		Use:   "create",
		Short: "Book a slot on an invitee's behalf (POST /invitees, Scheduling API; requires a paid plan)",
		Long: "Creates a real meeting on both calendars and Calendly emails the invitee a\n" +
			"confirmation at once — there is no draft or hold state, and undoing it\n" +
			"means `event cancel`, which emails them again.\n" +
			"\n" +
			"The Scheduling API needs a PAID Calendly plan; a free-tier account answers\n" +
			"403 and no retry changes that — mint a `link create` booking link instead.\n" +
			"`--start` is UTC ISO-8601 and must be a slot `availability slots` actually\n" +
			"offers, since an unavailable time is rejected. `--timezone` is the\n" +
			"INVITEE's IANA zone and decides what their confirmation says.\n" +
			"`--location-kind` must be one the event type permits (`event-type get`\n" +
			"lists them) with `--location` carrying the value, such as a phone number.\n" +
			"`--guest` repeats for additional attendees.",
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
