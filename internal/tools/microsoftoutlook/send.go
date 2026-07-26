package microsoftoutlook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newMessagesSendCmd(token string) *cobra.Command {
	var o composeOptions
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send an email (POST /me/sendMail)",
		Long: "`--to` and `--subject` are required, as is exactly one of `--body` /\n" +
			"`--body-file`. The body is plain text unless `--html` is set. `--to`,\n" +
			"`--cc` and `--bcc` each take a comma-separated list or repeat.\n" +
			"\n" +
			"`--attach` repeats and inlines each file as base64 in the message, so the\n" +
			"attachments total is capped at 3 MB and anything larger has to travel as a\n" +
			"shared link. The message is saved to Sent Items. Graph answers 202 Accepted\n" +
			"with an empty body, so success means accepted for delivery and NO message id\n" +
			"comes back — look in Sent Items if one is needed.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			bodyText, err := o.resolveComposeBody()
			if err != nil {
				return err
			}
			msg, err := buildGraphMessage(&o, bodyText)
			if err != nil {
				return err
			}
			payload := map[string]any{"message": msg, "saveToSentItems": true}
			// sendMail returns 202 Accepted with an empty body.
			if _, err := s.call(cmd.Context(), token, http.MethodPost, "/me/sendMail", nil, payload); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"status": "sent", "to": o.to})
			}
			fmt.Fprintln(s.stdout(), "sent message")
			return nil
		},
	}
	addAddressFlags(cmd, &o)
	addBodyFlags(cmd, &o)
	return cmd
}

func (s *Service) newMessagesReplyCmd(token string) *cobra.Command {
	var o composeOptions
	var replyAll bool
	cmd := &cobra.Command{
		Use:   "reply <message-id>",
		Short: "Reply to a message (sender only; --all for reply-all)",
		Long: "Threading, the subject and the quoted original all come from Graph;\n" +
			"`--body` (or `--body-file`) is only the new text above the quote. There are\n" +
			"no recipient flags — `--all` is the only way to widen it past the original\n" +
			"sender. Three calls run under the hood: a reply draft is created,\n" +
			"attachments are added, then it is sent, so a failure after the first leaves\n" +
			"an unsent draft behind in Drafts.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			bodyText, err := o.resolveComposeBody()
			if err != nil {
				return err
			}
			action := "createReply"
			if replyAll {
				action = "createReplyAll"
			}
			// createReply/createReplyAll returns a draft that already carries
			// the quoted original + threading; --comment adds the reply body.
			draftID, err := s.createDraftFrom(cmd.Context(), token, args[0], action, map[string]any{"comment": bodyText})
			if err != nil {
				return err
			}
			if err := s.addDraftAttachments(cmd.Context(), token, draftID, o.attachments); err != nil {
				return err
			}
			return s.sendDraft(cmd, token, draftID, "sent reply")
		},
	}
	addBodyFlags(cmd, &o)
	cmd.Flags().BoolVar(&replyAll, "all", false, "reply to all original recipients")
	return cmd
}

func (s *Service) newMessagesForwardCmd(token string) *cobra.Command {
	var to []string
	var preamble string
	cmd := &cobra.Command{
		Use:   "forward <message-id>",
		Short: "Forward a message with the original quoted",
		Long: "`--to` is required, and `--body` here is only a preamble ABOVE the quoted\n" +
			"original rather than a replacement for it. This verb has no `--body-file`,\n" +
			"`--html`, `--cc` or `--attach`; the original's own attachments travel with\n" +
			"the forward. Like `reply` it creates a draft and then sends it, so an\n" +
			"interrupted run can leave a draft in Drafts.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"toRecipients": recipients(to)}
			if preamble != "" {
				payload["comment"] = preamble
			}
			draftID, err := s.createDraftFrom(cmd.Context(), token, args[0], "createForward", payload)
			if err != nil {
				return err
			}
			return s.sendDraft(cmd, token, draftID, "forwarded message")
		},
	}
	cmd.Flags().StringSliceVar(&to, "to", nil, "recipient addresses (comma-separated or repeated)")
	cmd.Flags().StringVar(&preamble, "body", "", "optional preamble above the quoted message")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// createDraftFrom invokes a createReply/createReplyAll/createForward action and
// returns the id of the draft it produced.
func (s *Service) createDraftFrom(ctx context.Context, token, messageID, action string, payload map[string]any) (string, error) {
	body, err := s.call(ctx, token, http.MethodPost, "/me/messages/"+url.PathEscape(messageID)+"/"+action, nil, payload)
	if err != nil {
		return "", err
	}
	var draft struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &draft); err != nil {
		return "", fmt.Errorf("microsoft-outlook: decode %s response: %w", action, err)
	}
	if draft.ID == "" {
		return "", fmt.Errorf("microsoft-outlook: %s returned no draft id", action)
	}
	return draft.ID, nil
}

// addDraftAttachments uploads local files onto an existing draft message.
func (s *Service) addDraftAttachments(ctx context.Context, token, draftID string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	atts, err := fileAttachments(paths)
	if err != nil {
		return err
	}
	for _, att := range atts {
		if _, err := s.call(ctx, token, http.MethodPost, "/me/messages/"+url.PathEscape(draftID)+"/attachments", nil, att); err != nil {
			return err
		}
	}
	return nil
}

// sendDraft sends an existing draft by id and emits the result.
func (s *Service) sendDraft(cmd *cobra.Command, token, draftID, verb string) error {
	if _, err := s.call(cmd.Context(), token, http.MethodPost, "/me/messages/"+url.PathEscape(draftID)+"/send", nil, nil); err != nil {
		return err
	}
	if jsonOut(cmd) {
		return s.emitJSON(map[string]any{"status": "sent", "draftId": draftID})
	}
	fmt.Fprintf(s.stdout(), "%s (draft %s)\n", verb, draftID)
	return nil
}
