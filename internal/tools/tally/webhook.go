package tally

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newWebhookCmd(token string) *cobra.Command {
	cmd := newGroupCmd("webhook", "Webhooks (list, create, update, delete)")
	cmd.AddCommand(
		s.newWebhookListCmd(token),
		s.newWebhookCreateCmd(token),
		s.newWebhookUpdateCmd(token),
		s.newWebhookDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newWebhookListCmd(token string) *cobra.Command {
	var page, limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List webhooks (GET /webhooks)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if cmd.Flags().Changed("page") {
				q.Set("page", strconv.Itoa(page))
			}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", strconv.Itoa(limit))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/webhooks", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size")
	return cmd
}

func (s *Service) newWebhookCreateCmd(token string) *cobra.Command {
	var file string
	var stdin bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a webhook (POST /webhooks)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.readBody(file, stdin)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/webhooks", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	bodyFlags(cmd, &file, &stdin)
	return cmd
}

func (s *Service) newWebhookUpdateCmd(token string) *cobra.Command {
	var webhook, file string
	var stdin bool
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a webhook (PATCH /webhooks/{webhookId})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.readBody(file, stdin)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/webhooks/"+url.PathEscape(webhook), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&webhook, "webhook", "", "webhook id")
	bodyFlags(cmd, &file, &stdin)
	_ = cmd.MarkFlagRequired("webhook")
	return cmd
}

func (s *Service) newWebhookDeleteCmd(token string) *cobra.Command {
	var webhook string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a webhook (DELETE /webhooks/{webhookId})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/webhooks/"+url.PathEscape(webhook), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&webhook, "webhook", "", "webhook id")
	_ = cmd.MarkFlagRequired("webhook")
	return cmd
}
