package pinterest

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newAccountCmd(token string) *cobra.Command {
	cmd := newGroupCmd("account", "Authenticated Pinterest account")
	cmd.AddCommand(s.newAccountGetCmd(token))
	return cmd
}

func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get",
		Short:       "Get the authenticated account (GET /user_account)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/user_account", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
