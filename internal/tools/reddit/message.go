package reddit

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newMessageCmd is `reddit message send`: send a private message to a user.
func (s *Service) newMessageCmd(token string) *cobra.Command {
	cmd := newGroup("message", "Send private messages")
	cmd.AddCommand(s.newMessageSendCmd(token))
	return cmd
}

func (s *Service) newMessageSendCmd(token string) *cobra.Command {
	var to, subject, text string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a private message to a user",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if to == "" || subject == "" || text == "" {
				return &usageError{msg: "--to, --subject, and --text are required"}
			}
			form := url.Values{
				"api_type": {"json"},
				"to":       {to},
				"subject":  {subject},
				"text":     {text},
			}
			body, err := s.postForm(cmd.Context(), token, "/api/compose", form)
			if err != nil {
				return err
			}
			if _, err := checkJSONErrors(body); err != nil {
				return err
			}
			if jsonMode(cmd) {
				return s.emitValue(map[string]any{"sent": true, "to": to})
			}
			return s.emitLine("sent message to u/" + to)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "recipient username (required)")
	cmd.Flags().StringVar(&subject, "subject", "", "message subject (required)")
	cmd.Flags().StringVar(&text, "text", "", "message body markdown (required)")
	return cmd
}
