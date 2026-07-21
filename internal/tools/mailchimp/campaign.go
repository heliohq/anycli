package mailchimp

import (
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

// newCampaignCmd builds the campaign group: list, get, create, set-content,
// send, test, schedule, unschedule, delete.
func (s *Service) newCampaignCmd(r *requester) *cobra.Command {
	group := newGroupCmd("campaign", "Manage email campaigns")
	group.AddCommand(
		s.newCampaignListCmd(r),
		s.newCampaignGetCmd(r),
		s.newCampaignCreateCmd(r),
		s.newCampaignSetContentCmd(r),
		s.newCampaignActionCmd(r, "send", "Send a campaign", "/actions/send", nil),
		s.newCampaignTestCmd(r),
		s.newCampaignScheduleCmd(r),
		s.newCampaignActionCmd(r, "unschedule", "Unschedule a campaign", "/actions/unschedule", nil),
		s.newCampaignDeleteCmd(r),
	)
	return group
}

func (s *Service) newCampaignListCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns (GET /campaigns)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := listQuery(cmd)
			if status, _ := cmd.Flags().GetString("status"); status != "" {
				q.Set("status", status)
			}
			body, err := r.do(cmd.Context(), http.MethodGet, "/campaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd)
	cmd.Flags().String("status", "", "filter by status: save|paused|schedule|sending|sent")
	return cmd
}

func (s *Service) newCampaignGetCmd(r *requester) *cobra.Command {
	return &cobra.Command{
		Use:   "get <campaign_id>",
		Short: "Get one campaign (GET /campaigns/{campaign_id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := r.do(cmd.Context(), http.MethodGet, "/campaigns/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newCampaignCreateCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a regular campaign (POST /campaigns)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listID, _ := cmd.Flags().GetString("list")
			subject, _ := cmd.Flags().GetString("subject")
			fromName, _ := cmd.Flags().GetString("from-name")
			replyTo, _ := cmd.Flags().GetString("reply-to")
			if listID == "" || subject == "" || fromName == "" || replyTo == "" {
				return &usageError{msg: "campaign create requires --list, --subject, --from-name, and --reply-to"}
			}
			recipients := map[string]any{"list_id": listID}
			if segment, _ := cmd.Flags().GetString("segment"); segment != "" {
				recipients["segment_opts"] = map[string]any{"saved_segment_id": segment}
			}
			settings := map[string]any{
				"subject_line": subject,
				"from_name":    fromName,
				"reply_to":     replyTo,
			}
			if title, _ := cmd.Flags().GetString("title"); title != "" {
				settings["title"] = title
			}
			payload := map[string]any{
				"type":       "regular",
				"recipients": recipients,
				"settings":   settings,
			}
			body, err := r.do(cmd.Context(), http.MethodPost, "/campaigns", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().String("list", "", "audience (list) id (required)")
	cmd.Flags().String("segment", "", "saved segment id to target within the audience")
	cmd.Flags().String("subject", "", "subject line (required)")
	cmd.Flags().String("from-name", "", "from name (required)")
	cmd.Flags().String("reply-to", "", "reply-to email address (required)")
	cmd.Flags().String("title", "", "internal campaign title")
	return cmd
}

func (s *Service) newCampaignSetContentCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-content <campaign_id>",
		Short: "Set campaign content (PUT /campaigns/{campaign_id}/content)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			html, _ := cmd.Flags().GetString("html")
			htmlFile, _ := cmd.Flags().GetString("html-file")
			template, _ := cmd.Flags().GetString("template")
			plainText, _ := cmd.Flags().GetString("plain-text")

			set := 0
			for _, v := range []string{html, htmlFile, template} {
				if v != "" {
					set++
				}
			}
			if set != 1 {
				return &usageError{msg: "campaign set-content requires exactly one of --html, --html-file, or --template"}
			}
			payload := map[string]any{}
			switch {
			case htmlFile != "":
				b, err := os.ReadFile(htmlFile)
				if err != nil {
					return &usageError{msg: "cannot read --html-file: " + err.Error()}
				}
				payload["html"] = string(b)
			case html != "":
				payload["html"] = html
			case template != "":
				payload["template"] = map[string]any{"id": template}
			}
			if plainText != "" {
				payload["plain_text"] = plainText
			}
			body, err := r.do(cmd.Context(), http.MethodPut, "/campaigns/"+url.PathEscape(args[0])+"/content", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().String("html", "", "inline HTML content")
	cmd.Flags().String("html-file", "", "path to a file with the HTML content")
	cmd.Flags().String("template", "", "template id to render the campaign from")
	cmd.Flags().String("plain-text", "", "optional plain-text alternative")
	return cmd
}

// newCampaignActionCmd builds a no-body POST action command (send, unschedule)
// that emits a 204 receipt. buildPayload is nil for bodyless actions.
func (s *Service) newCampaignActionCmd(r *requester, action, short, subPath string, buildPayload func(cmd *cobra.Command) (any, error)) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <campaign_id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var payload any
			if buildPayload != nil {
				p, err := buildPayload(cmd)
				if err != nil {
					return err
				}
				payload = p
			}
			if _, err := r.do(cmd.Context(), http.MethodPost, "/campaigns/"+url.PathEscape(args[0])+subPath, nil, payload); err != nil {
				return err
			}
			return s.emitValue(actionReceipt(action, args[0]))
		},
	}
}

func (s *Service) newCampaignTestCmd(r *requester) *cobra.Command {
	cmd := s.newCampaignActionCmd(r, "test", "Send a test email (POST /campaigns/{campaign_id}/actions/test)", "/actions/test",
		func(cmd *cobra.Command) (any, error) {
			emails, _ := cmd.Flags().GetString("emails")
			list := splitCSV(emails)
			if len(list) == 0 {
				return nil, &usageError{msg: "campaign test requires --emails (comma-separated)"}
			}
			return map[string]any{"test_emails": list, "send_type": "html"}, nil
		})
	cmd.Flags().String("emails", "", "comma-separated test recipient addresses (required)")
	return cmd
}

func (s *Service) newCampaignScheduleCmd(r *requester) *cobra.Command {
	cmd := s.newCampaignActionCmd(r, "schedule", "Schedule a campaign (POST /campaigns/{campaign_id}/actions/schedule)", "/actions/schedule",
		func(cmd *cobra.Command) (any, error) {
			at, _ := cmd.Flags().GetString("at")
			if at == "" {
				return nil, &usageError{msg: "campaign schedule requires --at (RFC3339 timestamp)"}
			}
			return map[string]any{"schedule_time": at}, nil
		})
	cmd.Flags().String("at", "", "RFC3339 schedule time, e.g. 2026-08-01T15:00:00Z (required)")
	return cmd
}

func (s *Service) newCampaignDeleteCmd(r *requester) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <campaign_id>",
		Short: "Delete a campaign (DELETE /campaigns/{campaign_id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := r.do(cmd.Context(), http.MethodDelete, "/campaigns/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			return s.emitValue(actionReceipt("delete", args[0]))
		},
	}
}
