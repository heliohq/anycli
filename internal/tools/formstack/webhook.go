package formstack

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newWebhookCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "webhook", Short: "Webhooks (list, get, create, delete)"}
	cmd.AddCommand(
		s.newWebhookListCmd(token),
		s.newWebhookGetCmd(token),
		s.newWebhookCreateCmd(token),
		s.newWebhookDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newWebhookListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <form-id>",
		Short: "List a form's webhooks (GET /form/{id}/webhook.json)",
		Long: "Takes a FORM id: webhooks belong to a form and there is no account-wide\n" +
			"listing, so auditing where response data goes means walking `form list`\n" +
			"and calling this per form. Each entry's `id` is a webhook id — that, not\n" +
			"the form id, is what `webhook get` and `webhook delete` take.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form/"+url.PathEscape(args[0])+"/webhook.json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newWebhookGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <webhook-id>",
		Short: "Get a webhook (GET /webhook/{id}.json)",
		Long: "Takes the WEBHOOK id from `webhook list`, not the form id it is attached\n" +
			"to. Returns the target `url`, the `content_type` and the form it serves.\n" +
			"There is no update command anywhere in this tool: changing a URL is a\n" +
			"`webhook delete` plus a `webhook create`, and the replacement gets a new\n" +
			"id.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/webhook/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newWebhookCreateCmd(token string) *cobra.Command {
	var targetURL, contentType string
	cmd := &cobra.Command{
		Use:   "create <form-id>",
		Short: "Create a webhook on a form (POST /form/{id}/webhook.json)",
		Long: "Wires Formstack to POST to `--url` on every new submission of this form,\n" +
			"starting with the next one — respondent data leaves Formstack for that\n" +
			"endpoint from then on, with no confirmation step. `--content-type` is\n" +
			"`json` or `form`; omitting it leaves Formstack's own default. The payload\n" +
			"is keyed by field id, like every read here. Nothing validates the\n" +
			"endpoint at creation, so a wrong URL simply fails once per submission,\n" +
			"invisibly from this side.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"url": targetURL}
			if contentType != "" {
				body["content_type"] = contentType
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/form/"+url.PathEscape(args[0])+"/webhook.json", nil, body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&targetURL, "url", "", "URL to POST submissions to")
	cmd.Flags().StringVar(&contentType, "content-type", "", "payload encoding: json|form")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func (s *Service) newWebhookDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <webhook-id>",
		Short: "Delete a webhook (DELETE /webhook/{id}.json)",
		Long: "Takes the WEBHOOK id from `webhook list`, not the form id — passing the\n" +
			"form id here deletes someone else's webhook or fails. Forwarding stops\n" +
			"for submissions after the call; deliveries already dispatched are not\n" +
			"recalled. There is no disable-and-re-enable: restoring the flow means\n" +
			"`webhook create` again, with a new id.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/webhook/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
