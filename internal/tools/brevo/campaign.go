package brevo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCampaignListCmd builds `brevo campaign list` — GET /emailCampaigns.
func (s *Service) newCampaignListCmd(apiKey string) *cobra.Command {
	var (
		campaignType, status string
		limit, offset        int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List email campaigns (GET /emailCampaigns)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("limit", itoa(limit))
			q.Set("offset", itoa(offset))
			if campaignType != "" {
				q.Set("type", campaignType)
			}
			if status != "" {
				q.Set("status", status)
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/emailCampaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&campaignType, "type", "", "campaign type: classic|trigger")
	cmd.Flags().StringVar(&status, "status", "", "status filter: draft|sent|archive|queued|suspended|in_process")
	cmd.Flags().IntVar(&limit, "limit", 50, "page size")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}

// newCampaignGetCmd builds `brevo campaign get` — GET /emailCampaigns/{id}.
func (s *Service) newCampaignGetCmd(apiKey string) *cobra.Command {
	var id int
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get an email campaign (GET /emailCampaigns/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/emailCampaigns/"+itoa(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "campaign id (integer)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newCampaignCreateCmd builds `brevo campaign create` — POST /emailCampaigns.
// A scheduled bulk campaign (distinct from a one-off `email send`).
func (s *Service) newCampaignCreateCmd(apiKey string) *cobra.Command {
	var (
		name, subject, html, scheduledAt string
		senderEmail, senderName          string
		senderID, templateID             int
		listIDs                          []int
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an email campaign (POST /emailCampaigns)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name, "subject": subject}
			if cmd.Flags().Changed("sender-id") {
				body["sender"] = map[string]any{"id": senderID}
			} else if senderEmail != "" {
				sender := map[string]any{"email": senderEmail}
				if senderName != "" {
					sender["name"] = senderName
				}
				body["sender"] = sender
			}
			if html != "" {
				body["htmlContent"] = html
			}
			if cmd.Flags().Changed("template-id") {
				body["templateId"] = templateID
			}
			if len(listIDs) > 0 {
				body["recipients"] = map[string]any{"listIds": listIDs}
			}
			if scheduledAt != "" {
				body["scheduledAt"] = scheduledAt
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodPost, "/emailCampaigns", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "campaign name")
	cmd.Flags().StringVar(&subject, "subject", "", "campaign subject")
	cmd.Flags().StringVar(&senderEmail, "sender-email", "", "sender email (must be a verified sender)")
	cmd.Flags().StringVar(&senderName, "sender-name", "", "sender display name")
	cmd.Flags().IntVar(&senderID, "sender-id", 0, "verified sender id (alternative to --sender-email)")
	cmd.Flags().StringVar(&html, "html", "", "HTML body (htmlContent)")
	cmd.Flags().IntVar(&templateID, "template-id", 0, "template id (alternative to --html)")
	cmd.Flags().IntSliceVar(&listIDs, "list-ids", nil, "recipient list id (repeatable, integer)")
	cmd.Flags().StringVar(&scheduledAt, "scheduled-at", "", "ISO-8601 send time (omit for a draft)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("subject")
	return cmd
}
