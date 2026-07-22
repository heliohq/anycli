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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/verified_senders", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
