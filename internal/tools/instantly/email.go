package instantly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newEmailCmd(token string) *cobra.Command {
	cmd := newGroupCmd("email", "Unibox emails (list, get, reply, unread-count, mark-read)")
	cmd.AddCommand(
		s.newEmailListCmd(token),
		s.newEmailGetCmd(token),
		s.newEmailReplyCmd(token),
		s.newEmailUnreadCountCmd(token),
		s.newEmailMarkReadCmd(token),
	)
	return cmd
}

// newEmailListCmd wraps GET /emails. Note the provider caps this endpoint at 20
// req/min (tighter than the workspace-wide budget) — surfaced in the docs.
func (s *Service) newEmailListCmd(token string) *cobra.Command {
	var page pageFlags
	var search, campaignID, eaccount, isUnread string
	cmd := &cobra.Command{
		Use:         "list",
		Annotations: readOnly,
		Short:       "List Unibox emails (GET /emails; 20 req/min cap)",
		Long: "Rate-limited to 20 requests per minute, far below the workspace budget, so\n" +
			"this must not be polled in a tight loop. --is-unread takes the string\n" +
			"\"true\" or \"false\", not a bare flag. --campaign-id, --eaccount and\n" +
			"--search narrow further; cursor-paged with --limit and --starting-after.\n" +
			"The ids returned are the EMAIL ids `email reply --reply-to-uuid` takes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.applyQuery(q)
			setIfChanged(cmd, q, "search", "search", search)
			setIfChanged(cmd, q, "campaign-id", "campaign_id", campaignID)
			setIfChanged(cmd, q, "eaccount", "eaccount", eaccount)
			setIfChanged(cmd, q, "is-unread", "is_unread", isUnread)
			return s.get(cmd, token, "/emails", q)
		},
	}
	registerPageFlags(cmd, &page)
	cmd.Flags().StringVar(&search, "search", "", "free-text search")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "filter by campaign id")
	cmd.Flags().StringVar(&eaccount, "eaccount", "", "filter by sending account email")
	cmd.Flags().StringVar(&isUnread, "is-unread", "", "filter unread only (true/false)")
	return cmd
}

func (s *Service) newEmailGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Annotations: readOnly,
		Short:       "Get a single email (GET /emails/{id})",
		Long: "--id is one message's id. Two id spaces run through this group and they\n" +
			"are not interchangeable: this and `email reply --reply-to-uuid` take an\n" +
			"EMAIL id, while `email mark-read` takes a THREAD id.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/emails/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "email id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newEmailReplyCmd wraps POST /emails/reply. reply_to_uuid is the id of the
// email being replied to; eaccount is the sending account.
func (s *Service) newEmailReplyCmd(token string) *cobra.Command {
	var eaccount, replyToUUID, subject, body, cc, bcc string
	cmd := &cobra.Command{
		Use:         "reply",
		Annotations: writeAction,
		Short:       "Reply to an email in the Unibox (POST /emails/reply)",
		Long: "Sends a REAL email to a real prospect the moment it returns; there is no\n" +
			"draft state and nothing recalls it. --eaccount, --reply-to-uuid,\n" +
			"--subject and --body are all required: --eaccount is the sending account\n" +
			"the reply goes out from, and --reply-to-uuid is the id of the email being\n" +
			"replied to, from `email list` or `email get`. --body takes HTML or text;\n" +
			"--cc and --bcc are comma-separated address lists.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{
				"eaccount":      eaccount,
				"reply_to_uuid": replyToUUID,
				"subject":       subject,
				"body":          body,
			}
			if cmd.Flags().Changed("cc") {
				payload["cc_address_email_list"] = cc
			}
			if cmd.Flags().Changed("bcc") {
				payload["bcc_address_email_list"] = bcc
			}
			return s.send(cmd, token, http.MethodPost, "/emails/reply", payload)
		},
	}
	cmd.Flags().StringVar(&eaccount, "eaccount", "", "sending account email")
	cmd.Flags().StringVar(&replyToUUID, "reply-to-uuid", "", "id of the email being replied to")
	cmd.Flags().StringVar(&subject, "subject", "", "reply subject")
	cmd.Flags().StringVar(&body, "body", "", "reply body (HTML or text)")
	cmd.Flags().StringVar(&cc, "cc", "", "comma-separated CC addresses")
	cmd.Flags().StringVar(&bcc, "bcc", "", "comma-separated BCC addresses")
	_ = cmd.MarkFlagRequired("eaccount")
	_ = cmd.MarkFlagRequired("reply-to-uuid")
	_ = cmd.MarkFlagRequired("subject")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newEmailUnreadCountCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "unread-count",
		Annotations: readOnly,
		Short:       "Count unread Unibox emails (GET /emails/unread/count)",
		Long: "One number for the whole Unibox — there is no per-campaign or per-account\n" +
			"form, so a scoped count means running `email list --is-unread true` with\n" +
			"filters and counting the rows, against that command's 20-per-minute cap.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/emails/unread/count", nil)
		},
	}
	return cmd
}

func (s *Service) newEmailMarkReadCmd(token string) *cobra.Command {
	var threadID string
	cmd := &cobra.Command{
		Use:         "mark-read",
		Annotations: writeAction,
		Short:       "Mark a thread as read (POST /emails/threads/{thread_id}/mark-as-read)",
		Long: "--thread-id is a THREAD id, not the email id `email get` and `email reply`\n" +
			"take; the two id spaces are distinct and passing one for the other fails.\n" +
			"This marks the whole thread and moves the figure `email unread-count`\n" +
			"reports. It sends nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.send(cmd, token, http.MethodPost, "/emails/threads/"+url.PathEscape(threadID)+"/mark-as-read", nil)
		},
	}
	cmd.Flags().StringVar(&threadID, "thread-id", "", "thread id")
	_ = cmd.MarkFlagRequired("thread-id")
	return cmd
}
