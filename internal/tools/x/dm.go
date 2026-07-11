package x

import "github.com/spf13/cobra"

type dmAttachment struct {
	MediaID string `json:"media_id"`
}

type dmMessage struct {
	Text        string         `json:"text,omitempty"`
	Attachments []dmAttachment `json:"attachments,omitempty"`
}

func (s *Service) newDMCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "dm", Short: "Legacy Direct Messages (not XChat)"}
	cmd.AddCommand(
		s.newDMListCmd(token),
		s.newDMGetCmd(token),
		s.newDMHistoryCmd(token),
		s.newDMSendCmd(token),
		s.newDMDeleteCmd(token),
		s.newDMConversationCmd(token),
		s.newDMMediaCmd(token),
	)
	return cmd
}

func buildDMMessage(text, mediaID string) (dmMessage, error) {
	if text == "" && mediaID == "" {
		return dmMessage{}, requireDMContentError()
	}
	message := dmMessage{Text: text}
	if mediaID != "" {
		if err := requireNumericID("media id", mediaID); err != nil {
			return dmMessage{}, err
		}
		message.Attachments = []dmAttachment{{MediaID: mediaID}}
	}
	return message, nil
}
