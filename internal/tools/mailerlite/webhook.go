package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newWebhookCmd builds the `mailerlite webhook` command tree — register and
// inspect event callbacks.
func (s *Service) newWebhookCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "webhook", Short: "Webhooks (list, get, create, update, delete)"}
	cmd.AddCommand(
		s.newWebhookListCmd(token),
		s.newWebhookGetCmd(token),
		s.newWebhookCreateCmd(token),
		s.newWebhookUpdateCmd(token),
		s.newWebhookDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newWebhookListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List webhooks (GET /webhooks)",
		Long: "Returns every webhook on the account in one response — there are no\n" +
			"pagination flags here at all. Check this before creating another one:\n" +
			"duplicate subscriptions to the same event both fire, doubling downstream\n" +
			"processing.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/webhooks", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newWebhookGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a webhook (GET /webhooks/{id})",
		Long: "Reads one webhook's callback URL, its enabled state and the exact event\n" +
			"list it subscribes to — worth confirming before `webhook update --events`,\n" +
			"which replaces that list rather than extending it.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/webhooks/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newWebhookCreateCmd(token string) *cobra.Command {
	var callbackURL, name, events string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook (POST /webhooks)",
		Long: "--url must be publicly reachable by MailerLite, which will call it as\n" +
			"events occur; a local or unreachable address fails at delivery time rather\n" +
			"than here. --events is a comma-separated list of event names such as\n" +
			"subscriber.created or subscriber.updated, and both flags are required.\n" +
			"Registering a webhook has no effect on data — it only starts a stream of\n" +
			"outbound calls.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"url":    callbackURL,
				"events": splitList(events),
			}
			if cmd.Flags().Changed("name") {
				body["name"] = name
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/webhooks", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&callbackURL, "url", "", "callback URL that receives events (required)")
	cmd.Flags().StringVar(&events, "events", "", "comma-separated event names, e.g. subscriber.created (required)")
	cmd.Flags().StringVar(&name, "name", "", "webhook name")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("events")
	return cmd
}

func (s *Service) newWebhookUpdateCmd(token string) *cobra.Command {
	var callbackURL, name, events string
	var enabled bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a webhook (PUT /webhooks/{id})",
		Long: "Only the flags passed are sent, so omitted settings survive — but --events\n" +
			"REPLACES the whole event list rather than adding to it, so subscribing to\n" +
			"one more event means listing the existing ones again. --enabled=false\n" +
			"pauses delivery while keeping the registration, which is the reversible\n" +
			"alternative to `webhook delete`.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairs := []bodyField{
				{key: "url", value: callbackURL, set: cmd.Flags().Changed("url")},
				{key: "name", value: name, set: cmd.Flags().Changed("name")},
				{key: "enabled", value: enabled, set: cmd.Flags().Changed("enabled")},
			}
			if cmd.Flags().Changed("events") {
				pairs = append(pairs, bodyField{key: "events", value: splitList(events), set: true})
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/webhooks/"+url.PathEscape(args[0]), nil, buildBody(pairs))
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&callbackURL, "url", "", "callback URL")
	cmd.Flags().StringVar(&events, "events", "", "comma-separated event names")
	cmd.Flags().StringVar(&name, "name", "", "webhook name")
	cmd.Flags().BoolVar(&enabled, "enabled", false, "enable or disable the webhook")
	return cmd
}

func (s *Service) newWebhookDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a webhook (DELETE /webhooks/{id})",
		Long: "Permanent, and delivery stops at once — events that occur afterwards are\n" +
			"not queued or replayed once a replacement is registered. To stop delivery\n" +
			"reversibly use `webhook update --enabled=false` instead.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/webhooks/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
