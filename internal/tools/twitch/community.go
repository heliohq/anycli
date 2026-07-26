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
		Use:   "list",
		Short: "List a channel's followers (self by default)",
		Long: "Requires the moderator:read:followers scope AND the connected account\n" +
			"being the broadcaster or a moderator of the target channel — reading a\n" +
			"channel where it holds neither role returns 401 or 403 rather than a\n" +
			"partial list. Defaults to the connected account's own channel.\n" +
			"\n" +
			"--user-id turns this into a membership check for one person: does this\n" +
			"user follow the channel, answered in a single call instead of paging every\n" +
			"follower.",
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
		Use:   "list",
		Short: "List your channel's subscribers",
		Long: "Always the connected account's own channel — there is deliberately no\n" +
			"--broadcaster-id flag, because Twitch lets a broadcaster read nobody\n" +
			"else's subscriptions. Requires the channel:read:subscriptions scope.\n" +
			"--first is capped at 100; continue with --after.",
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
		Use:   "send",
		Short: "Send a chat message to a channel (self by default)",
		Long: "Posts publicly and immediately as the connected account, into its own\n" +
			"channel by default or another with --broadcaster-id. There is no edit or\n" +
			"delete here. --reply-parent-message-id threads the message under an\n" +
			"existing one.\n" +
			"\n" +
			"Twitch can refuse the send even with the user:write:chat scope granted:\n" +
			"account phone verification, the channel's own chat settings\n" +
			"(followers-only, subscriber-only, slow mode) and per-account rate limits\n" +
			"all apply. Such a refusal is a rule rather than a transient failure, so\n" +
			"the Helix `message` in the error is the thing to read — retrying the same\n" +
			"call will be refused again.",
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
		Use:   "chatters",
		Short: "List users connected to a channel's chat (self by default)",
		Long: "Who is CONNECTED to the chat right now, not who has spoken — the list\n" +
			"turns over constantly, so it is a snapshot rather than an audience roster.\n" +
			"Requires the moderator:read:chatters scope, and the moderator identity\n" +
			"sent is always the connected account, so another channel only works where\n" +
			"that account is its broadcaster or a moderator. --first is capped at 100;\n" +
			"continue with --after.",
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
