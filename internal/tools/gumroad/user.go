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
		Use:   "get",
		Short: "Get the authenticated user (GET /user)",
		Long: "Identifies the creator account behind the token — id, name, email, profile\n" +
			"URL and the currency every amount elsewhere is denominated in. The cheapest\n" +
			"connection check, and the way to confirm WHICH store a write is about to\n" +
			"land on when more than one Gumroad account is connected.",
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
