package loops

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newEventCmd groups event operations. Firing an event triggers any Loops
// workflows listening for it.
func (s *Service) newEventCmd(key string) *cobra.Command {
	cmd := newGroup("event", "Events (workflow triggers)")
	cmd.AddCommand(s.newEventSendCmd(key))
	return cmd
}

func (s *Service) newEventSendCmd(key string) *cobra.Command {
	var eventName, email, userID, propsJSON, idempotencyKey string
	var property, mailingList []string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send an event to trigger workflows (POST /v1/events/send)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"eventName": eventName}
			if email != "" {
				body["email"] = email
			}
			if userID != "" {
				body["userId"] = userID
			}
			eventProps := map[string]any{}
			if propsJSON != "" {
				raw, err := decodeJSONObject("event-properties-json", propsJSON)
				if err != nil {
					return err
				}
				for k, v := range raw {
					eventProps[k] = v
				}
			}
			kv, err := parseKeyValues("event-property", property)
			if err != nil {
				return err
			}
			for k, v := range kv {
				eventProps[k] = v
			}
			if len(eventProps) > 0 {
				body["eventProperties"] = eventProps
			}
			mailing, err := parseMailingLists(mailingList)
			if err != nil {
				return err
			}
			if mailing != nil {
				body["mailingLists"] = mailing
			}
			resp, err := s.callIdempotent(cmd.Context(), key, http.MethodPost, "/v1/events/send", nil, body, idempotencyKey)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&eventName, "event-name", "", "event name (required)")
	cmd.Flags().StringVar(&email, "email", "", "contact email address (required if --user-id is absent)")
	cmd.Flags().StringVar(&userID, "user-id", "", "external unique user id (required if --email is absent)")
	cmd.Flags().StringArrayVar(&property, "event-property", nil, "event property key=value, typed-coerced (repeatable)")
	cmd.Flags().StringVar(&propsJSON, "event-properties-json", "", "event properties as a raw JSON object")
	cmd.Flags().StringArrayVar(&mailingList, "mailing-list", nil, "mailing-list subscription id=true|false (repeatable)")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Idempotency-Key header (409 on replay)")
	_ = cmd.MarkFlagRequired("event-name")
	cmd.MarkFlagsOneRequired("email", "user-id")
	return cmd
}
