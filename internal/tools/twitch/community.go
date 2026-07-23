package twitch

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newFollowerListCmd lists a channel's followers (self by default). Requires the
// moderator:read:followers scope; the caller must be the broadcaster or a
// moderator of the channel to see the full list.
func (s *Service) newFollowerListCmd(rc *reqCtx) *cobra.Command {
	var (
		broadcasterID string
		userID        string
		page          paginationFlags
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a channel's followers (self by default)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := s.broadcasterOrSelf(cmd.Context(), rc, broadcasterID)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("broadcaster_id", id)
			if userID != "" {
				q.Set("user_id", userID)
			}
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/channels/followers", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().StringVar(&broadcasterID, "broadcaster-id", "", "target channel's broadcaster id (default: self)")
	cmd.Flags().StringVar(&userID, "user-id", "", "check whether this specific user follows the channel")
	registerPaginationFlags(cmd, &page)
	return cmd
}

// newSubscriberListCmd lists the caller's channel subscribers. Requires the
// channel:read:subscriptions scope; a broadcaster can only read its own
// subscriptions, so broadcaster_id is always self.
func (s *Service) newSubscriberListCmd(rc *reqCtx) *cobra.Command {
	var page paginationFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List your channel's subscribers",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := s.resolveSelfID(cmd.Context(), rc)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("broadcaster_id", id)
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/subscriptions", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	registerPaginationFlags(cmd, &page)
	return cmd
}

// chatSendPayload is the POST /chat/messages body.
type chatSendPayload struct {
	BroadcasterID    string `json:"broadcaster_id"`
	SenderID         string `json:"sender_id"`
	Message          string `json:"message"`
	ReplyParentMsgID string `json:"reply_parent_message_id,omitempty"`
}

// newChatSendCmd sends a chat message to a channel (self by default). Requires
// the user:write:chat scope; sender_id is always the authenticated user.
func (s *Service) newChatSendCmd(rc *reqCtx) *cobra.Command {
	var (
		broadcasterID  string
		message        string
		replyParentMsg string
	)
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a chat message to a channel (self by default)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if message == "" {
				return &usageError{msg: "twitch: chat send requires --message"}
			}
			senderID, err := s.resolveSelfID(cmd.Context(), rc)
			if err != nil {
				return err
			}
			target := broadcasterID
			if target == "" {
				target = senderID
			}
			payload := chatSendPayload{
				BroadcasterID:    target,
				SenderID:         senderID,
				Message:          message,
				ReplyParentMsgID: replyParentMsg,
			}
			body, err := s.call(cmd.Context(), rc, http.MethodPost, "/chat/messages", nil, payload)
			if err != nil {
				return err
			}
			return s.emitOne(body)
		},
	}
	cmd.Flags().StringVar(&broadcasterID, "broadcaster-id", "", "target channel's broadcaster id (default: self)")
	cmd.Flags().StringVar(&message, "message", "", "message text (required)")
	cmd.Flags().StringVar(&replyParentMsg, "reply-parent-message-id", "", "message id to reply to")
	return cmd
}

// newChattersCmd lists the users connected to a channel's chat (self by
// default). Requires the moderator:read:chatters scope; moderator_id is always
// the authenticated user.
func (s *Service) newChattersCmd(rc *reqCtx) *cobra.Command {
	var (
		broadcasterID string
		page          paginationFlags
	)
	cmd := &cobra.Command{
		Use:         "chatters",
		Short:       "List users connected to a channel's chat (self by default)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			moderatorID, err := s.resolveSelfID(cmd.Context(), rc)
			if err != nil {
				return err
			}
			target := broadcasterID
			if target == "" {
				target = moderatorID
			}
			q := url.Values{}
			q.Set("broadcaster_id", target)
			q.Set("moderator_id", moderatorID)
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/chat/chatters", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().StringVar(&broadcasterID, "broadcaster-id", "", "target channel's broadcaster id (default: self)")
	registerPaginationFlags(cmd, &page)
	return cmd
}
