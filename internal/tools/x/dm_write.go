package x

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newDMConversationCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "conversation", Short: "DM conversations"}
	cmd.AddCommand(s.newDMConversationCreateCmd(token))
	return cmd
}

func (s *Service) newDMConversationCreateCmd(token string) *cobra.Command {
	var participantIDs []string
	var text, mediaID string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a group DM conversation with its first message",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(participantIDs) < 2 || len(participantIDs) > 49 {
				return fmt.Errorf("between 2 and 49 --participant-id values are required for a group conversation")
			}
			for _, participantID := range participantIDs {
				if err := requireNumericID("participant id", participantID); err != nil {
					return err
				}
			}
			message, err := buildDMMessage(text, mediaID)
			if err != nil {
				return err
			}
			payload := struct {
				ConversationType string    `json:"conversation_type"`
				ParticipantIDs   []string  `json:"participant_ids"`
				Message          dmMessage `json:"message"`
			}{
				ConversationType: "Group",
				ParticipantIDs:   participantIDs,
				Message:          message,
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/2/dm_conversations", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&participantIDs, "participant-id", nil, "participant user id, excluding yourself (repeat 2-49 times)")
	cmd.Flags().StringVar(&text, "text", "", "first message text")
	cmd.Flags().StringVar(&mediaID, "media-id", "", "one uploaded dm_image media id")
	return cmd
}

func (s *Service) newDMSendCmd(token string) *cobra.Command {
	var conversationID, participantID, text, mediaID string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a message by conversation id or participant id",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireExactlyOne("--conversation-id", conversationID, "--participant-id", participantID); err != nil {
				return err
			}
			if conversationID != "" {
				if err := requireDMConversationID(conversationID); err != nil {
					return err
				}
			}
			message, err := buildDMMessage(text, mediaID)
			if err != nil {
				return err
			}
			path := "/2/dm_conversations/" + url.PathEscape(conversationID) + "/messages"
			if participantID != "" {
				if err := requireNumericID("participant id", participantID); err != nil {
					return err
				}
				path = "/2/dm_conversations/with/" + url.PathEscape(participantID) + "/messages"
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, message)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&conversationID, "conversation-id", "", "existing DM conversation id")
	cmd.Flags().StringVar(&participantID, "participant-id", "", "participant user id for a one-to-one DM")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&mediaID, "media-id", "", "one uploaded dm_image media id")
	return cmd
}

func (s *Service) newDMDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <event-id>",
		Short:       "Delete a DM event sent by the connected user",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("DM event id", args[0]); err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodDelete, "/2/dm_events/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func requireDMContentError() error {
	return fmt.Errorf("at least one of --text or --media-id is required")
}
