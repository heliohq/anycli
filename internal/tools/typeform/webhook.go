package typeform

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newWebhookListCmd is `webhook list <form_id>`
// (GET /forms/{form_id}/webhooks): all webhooks configured on a form. Output
// JSON.
func (s *Service) newWebhookListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <form_id>",
		Short: "List a form's webhooks (GET /forms/{form_id}/webhooks)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/forms/"+url.PathEscape(args[0])+"/webhooks", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}

// newWebhookGetCmd is `webhook get <form_id> <tag>`
// (GET /forms/{form_id}/webhooks/{tag}): one webhook by its tag. Output JSON.
func (s *Service) newWebhookGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <form_id> <tag>",
		Short: "Retrieve a single webhook (GET /forms/{form_id}/webhooks/{tag})",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/forms/"+url.PathEscape(args[0])+"/webhooks/"+url.PathEscape(args[1]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}

// newWebhookSetCmd is `webhook set <form_id> <tag> --url u [...]`
// (PUT /forms/{form_id}/webhooks/{tag}): create-or-update by tag (the API's
// upsert semantics). --event-types passes a raw JSON object through verbatim
// (e.g. '{"form_response_partial":true}'). Output JSON (the created/updated
// webhook).
func (s *Service) newWebhookSetCmd(token string) *cobra.Command {
	var webhookURL, secret, eventTypes string
	var enabled, verifySSL bool
	cmd := &cobra.Command{
		Use:   "set <form_id> <tag>",
		Short: "Create or update a webhook (PUT /forms/{form_id}/webhooks/{tag})",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"url": webhookURL}
			if cmd.Flags().Changed("enabled") {
				payload["enabled"] = enabled
			}
			if cmd.Flags().Changed("verify-ssl") {
				payload["verify_ssl"] = verifySSL
			}
			if secret != "" {
				payload["secret"] = secret
			}
			if eventTypes != "" {
				et, err := readJSONArg("event-types", eventTypes)
				if err != nil {
					return err
				}
				payload["event_types"] = et
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut,
				"/forms/"+url.PathEscape(args[0])+"/webhooks/"+url.PathEscape(args[1]), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&webhookURL, "url", "", "destination URL for form submissions (required)")
	cmd.Flags().BoolVar(&enabled, "enabled", false, "deliver responses to the webhook")
	cmd.Flags().BoolVar(&verifySSL, "verify-ssl", false, "verify SSL certificates when delivering")
	cmd.Flags().StringVar(&secret, "secret", "", "HMAC-SHA256 signing secret")
	cmd.Flags().StringVar(&eventTypes, "event-types", "", "event-types JSON object, inline or @file")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

// newWebhookDeleteCmd is `webhook delete <form_id> <tag>`
// (DELETE /forms/{form_id}/webhooks/{tag}). Success is 204 No Content; a
// client-side receipt is emitted. Output JSON.
func (s *Service) newWebhookDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <form_id> <tag>",
		Short: "Delete a webhook (DELETE /forms/{form_id}/webhooks/{tag})",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete,
				"/forms/"+url.PathEscape(args[0])+"/webhooks/"+url.PathEscape(args[1]), nil, nil); err != nil {
				return err
			}
			return s.emitOK(map[string]any{"deleted": true, "form_id": args[0], "tag": args[1]})
		},
	}
	return cmd
}
