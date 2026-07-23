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
		Use:         "list <form-id>",
		Short:       "List a form's webhooks (GET /form/{id}/webhook.json)",
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
		Use:         "get <webhook-id>",
		Short:       "Get a webhook (GET /webhook/{id}.json)",
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
		Use:         "create <form-id>",
		Short:       "Create a webhook on a form (POST /form/{id}/webhook.json)",
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
		Use:         "delete <webhook-id>",
		Short:       "Delete a webhook (DELETE /webhook/{id}.json)",
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
