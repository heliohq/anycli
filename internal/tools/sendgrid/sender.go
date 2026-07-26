package sendgrid

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newSenderCmd(token string, region *string) *cobra.Command {
	cmd := &cobra.Command{Use: "sender", Short: "Verified sender identities (list)"}
	cmd.AddCommand(s.newSenderListCmd(token, region))
	return cmd
}

// newSenderListCmd wraps GET /v3/verified_senders: the verified sender
// identities that are valid `from` addresses for `mail send`.
func (s *Service) newSenderListCmd(token string, region *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List verified sender identities (GET /v3/verified_senders)",
		Long: "These addresses are the only values `mail send --from` accepts. Check\n" +
			"each entry's `verified` flag: a pending identity is still listed but\n" +
			"cannot send yet. Creating one is not possible from here — the account\n" +
			"owner adds the address in the SendGrid UI and clicks the confirmation\n" +
			"link sent to it.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/verified_senders", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
