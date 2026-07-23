package front

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMessageSendCmd is `message send --conversation <cnv_id> --body <text>`
// (POST /conversations/{id}/messages) — send an outbound reply into an existing
// conversation. --channel and --author are optional (Front sends from the
// conversation's channel and as the app/token by default); --text supplies a
// plain-text alternative body and --to overrides recipients. This replies to an
// existing thread; it never starts a new conversation.
func (s *Service) newMessageSendCmd(token string) *cobra.Command {
	var conversation, body, text, author, channel string
	var to []string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Reply into an existing conversation",
		Args:  cobra.NoArgs,
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&conversation, "conversation", "", "conversation id to reply into (required)")
	cmd.Flags().StringVar(&body, "body", "", "message body (required)")
	cmd.Flags().StringVar(&text, "text", "", "plain-text alternative body")
	cmd.Flags().StringVar(&author, "author", "", "teammate id to send on behalf of")
	cmd.Flags().StringVar(&channel, "channel", "", "channel id to send from")
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient handle (repeatable)")
	_ = cmd.MarkFlagRequired("conversation")
	_ = cmd.MarkFlagRequired("body")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{"body": body}
		if text != "" {
			payload["text"] = text
		}
		if author != "" {
			payload["author_id"] = author
		}
		if channel != "" {
			payload["channel_id"] = channel
		}
		if len(to) > 0 {
			payload["to"] = to
		}
		resp, err := s.call(cmd.Context(), token, http.MethodPost,
			"/conversations/"+url.PathEscape(conversation)+"/messages", nil, payload)
		if err != nil {
			return err
		}
		return s.emitObject(resp)
	}
	return cmd
}

// newDraftCreateCmd is `draft create --conversation <cnv_id> --body <text>
// --channel <channel_id>` (POST /conversations/{id}/drafts) — draft a reply for
// a human to review and send, the safer default over auto-sending. Front
// requires channel_id on a draft; --author (teammate) is optional and --to
// overrides recipients.
func (s *Service) newDraftCreateCmd(token string) *cobra.Command {
	var conversation, body, channel, author, subject string
	var to []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a draft reply for a human to review",
		Args:  cobra.NoArgs,
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&conversation, "conversation", "", "conversation id to draft into (required)")
	cmd.Flags().StringVar(&body, "body", "", "draft body (required)")
	cmd.Flags().StringVar(&channel, "channel", "", "channel id the draft will be sent from (required)")
	cmd.Flags().StringVar(&author, "author", "", "teammate id to draft on behalf of")
	cmd.Flags().StringVar(&subject, "subject", "", "draft subject (email channels)")
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient handle (repeatable)")
	_ = cmd.MarkFlagRequired("conversation")
	_ = cmd.MarkFlagRequired("body")
	_ = cmd.MarkFlagRequired("channel")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{"body": body, "channel_id": channel}
		if author != "" {
			payload["author_id"] = author
		}
		if subject != "" {
			payload["subject"] = subject
		}
		if len(to) > 0 {
			payload["to"] = to
		}
		resp, err := s.call(cmd.Context(), token, http.MethodPost,
			"/conversations/"+url.PathEscape(conversation)+"/drafts", nil, payload)
		if err != nil {
			return err
		}
		return s.emitObject(resp)
	}
	return cmd
}

// newCommentAddCmd is `comment add --conversation <cnv_id> --body <text>`
// (POST /conversations/{id}/comments) — leave an internal note; @mention a
// teammate inside the body. --author (teammate id) is passed when supplied.
func (s *Service) newCommentAddCmd(token string) *cobra.Command {
	var conversation, body, author string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an internal comment to a conversation",
		Args:  cobra.NoArgs,
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&conversation, "conversation", "", "conversation id to comment on (required)")
	cmd.Flags().StringVar(&body, "body", "", "comment body (required)")
	cmd.Flags().StringVar(&author, "author", "", "teammate id creating the comment")
	_ = cmd.MarkFlagRequired("conversation")
	_ = cmd.MarkFlagRequired("body")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{"body": body}
		if author != "" {
			payload["author_id"] = author
		}
		resp, err := s.call(cmd.Context(), token, http.MethodPost,
			"/conversations/"+url.PathEscape(conversation)+"/comments", nil, payload)
		if err != nil {
			return err
		}
		return s.emitObject(resp)
	}
	return cmd
}
