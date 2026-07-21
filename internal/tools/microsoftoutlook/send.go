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
		Use:         "send",
		Short:       "Send an email (POST /me/sendMail)",
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
		Use:         "reply <message-id>",
		Short:       "Reply to a message (sender only; --all for reply-all)",
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
		Use:         "forward <message-id>",
		Short:       "Forward a message with the original quoted",
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
