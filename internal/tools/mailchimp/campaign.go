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
		s.newCampaignActionCmd(r, "send", "Send a campaign", longCampaignSend, "/actions/send", nil),
		s.newCampaignTestCmd(r),
		s.newCampaignScheduleCmd(r),
		s.newCampaignActionCmd(r, "unschedule", "Unschedule a campaign", longCampaignUnschedule, "/actions/unschedule", nil),
		s.newCampaignDeleteCmd(r),
	)
	return group
}

func (s *Service) newCampaignListCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns (GET /campaigns)",
		Long: "`--status` narrows to `save` (draft), `paused`, `schedule`, `sending` or\n" +
			"`sent`. `save` is the one to use when hunting for a draft left behind by an\n" +
			"earlier `campaign create` whose id was lost. A campaign keeps its id forever\n" +
			"once sent, and that same id is the key `report get` takes.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "Returns `status`, the `recipients` block (the list id and segment frozen at\n" +
			"creation) and `settings` — but NOT the body, which lives behind a separate\n" +
			"content endpoint this tool only writes to, via `campaign set-content`. Read\n" +
			"`status` here before acting: only a `save` campaign can be sent or scheduled,\n" +
			"and on a `sent` one `emails_sent` is the real delivery count.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
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
		Long: "Creates an EMPTY campaign of type `regular` and returns its id. It has no\n" +
			"body yet, so sending or scheduling it now fails — follow immediately with\n" +
			"`campaign set-content`. `--list`, `--subject`, `--from-name` and `--reply-to`\n" +
			"are all required. The audience and the optional `--segment` are frozen here;\n" +
			"retargeting means creating another campaign. `--title` is an internal label\n" +
			"only and is never shown to recipients.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
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
		Long: "Requires exactly one of `--html`, `--html-file` or `--template`; zero or two\n" +
			"is a usage error raised before the call. `--html-file` is read locally and\n" +
			"inlined, making it the same request as a large `--html`. `--template` renders\n" +
			"from a saved template id (see `template list`). `--plain-text` is optional —\n" +
			"without it Mailchimp generates the text alternative itself. Re-running\n" +
			"replaces the body wholesale, which is fine while the campaign is still a\n" +
			"draft.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
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

// longCampaignSend, longCampaignUnschedule, longCampaignTest and
// longCampaignSchedule are the four action Longs. They live next to the shared
// builder because it is the builder that fixes the campaign-id argument and the
// 204-receipt response shape all four describe.
const (
	longCampaignSend = "Delivers to the audience and segment frozen at `campaign create` time; the\n" +
		"campaign must already have a body from `campaign set-content` or the send\n" +
		"fails. There is no recall and no per-recipient undo — `campaign unschedule`\n" +
		"only helps while a campaign is still scheduled. Prefer one\n" +
		"`campaign test --emails` first. Mailchimp answers 204, so success prints\n" +
		"`{\"ok\":true,\"action\":\"send\",\"id\":...}` rather than the campaign."

	longCampaignUnschedule = "Returns a scheduled campaign to draft (`save`) status, freeing it for another\n" +
		"`campaign schedule` or an immediate `campaign send`. It only applies while\n" +
		"the campaign's status is `schedule`; once Mailchimp starts sending, nothing\n" +
		"here stops it. Check `campaign get` for the current status if unsure."

	longCampaignTest = "`--emails` is a comma-separated list of addresses that need not be members of\n" +
		"the audience. The test always goes out as the HTML version, and the campaign\n" +
		"must already have content. A test does not change the campaign's status,\n" +
		"does not count against its report, and can be repeated — it is the only\n" +
		"rehearsal available before an irreversible `campaign send`."

	longCampaignSchedule = "`--at` is an RFC3339 timestamp that must be in the future AND on a quarter-hour\n" +
		"boundary — Mailchimp accepts only :00, :15, :30 and :45, and rejects anything\n" +
		"else rather than rounding. Scheduling requires content to be set, exactly as\n" +
		"sending does. Reversible with `campaign unschedule` up until the send starts,\n" +
		"which makes it the safer of the two outward paths."
)

// newCampaignActionCmd builds a no-body POST action command (send, unschedule)
// that emits a 204 receipt. buildPayload is nil for bodyless actions.
func (s *Service) newCampaignActionCmd(r *requester, action, short, long, subPath string, buildPayload func(cmd *cobra.Command) (any, error)) *cobra.Command {
	return &cobra.Command{
		Use:         action + " <campaign_id>",
		Short:       short,
		Long:        long,
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
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
	cmd := s.newCampaignActionCmd(r, "test", "Send a test email (POST /campaigns/{campaign_id}/actions/test)", longCampaignTest, "/actions/test",
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
	cmd := s.newCampaignActionCmd(r, "schedule", "Schedule a campaign (POST /campaigns/{campaign_id}/actions/schedule)", longCampaignSchedule, "/actions/schedule",
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
		Long: "Deletes the campaign object itself, and Mailchimp answers 204, so success\n" +
			"prints `{\"ok\":true,\"action\":\"delete\",\"id\":...}` rather than a resource. A SENT\n" +
			"campaign cannot be unsent by this — the emails are already delivered — and\n" +
			"deleting it also destroys its report, so the performance data disappears with\n" +
			"the record. To stop a campaign that has not gone out yet, `campaign\n" +
			"unschedule` is the reversible move.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := r.do(cmd.Context(), http.MethodDelete, "/campaigns/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			return s.emitValue(actionReceipt("delete", args[0]))
		},
	}
}
