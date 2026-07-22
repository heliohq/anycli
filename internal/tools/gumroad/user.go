package gumroad

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newUserCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Authenticated Gumroad user"}
	cmd.AddCommand(s.newUserGetCmd(token))
	return cmd
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get",
		Short:       "Get the authenticated user (GET /user)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET /user
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/user", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
