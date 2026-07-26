package calendly

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newLinkCreateCmd wraps POST /scheduling_links to mint a single-use booking
// link for an event type (body {max_event_count, owner: <event_type URI>,
// owner_type: "EventType"}). The response carries booking_url — the link to
// share.
func (s *Service) newLinkCreateCmd(token string) *cobra.Command {
	var eventType string
	var maxEventCount int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint a single-use scheduling link for an event type (POST /scheduling_links)",
		Long: "Returns `booking_url`, a link that stops accepting bookings after\n" +
			"`--max-event-count` uses — 1 by default, which is what makes it single-use.\n" +
			"It offers one person a slot without handing out the event type's permanent\n" +
			"`scheduling_url`, and it is the fallback when `book create` is refused for\n" +
			"want of a paid plan. The link is live the moment it is minted and there is\n" +
			"no command here to revoke it.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"max_event_count": maxEventCount,
				"owner":           s.normalizeURI("event_types", eventType),
				"owner_type":      "EventType",
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/scheduling_links", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&eventType, "event-type", "", "event type URI or bare UUID to book against")
	cmd.Flags().IntVar(&maxEventCount, "max-event-count", 1, "how many bookings the link allows (single-use = 1)")
	_ = cmd.MarkFlagRequired("event-type")
	return cmd
}
