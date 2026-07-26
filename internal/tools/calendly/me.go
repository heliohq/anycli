package calendly

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Current user: URI, organization URI, scheduling_url, timezone (GET /users/me)",
		Long: "One call that yields the inputs everything else needs: the user URI, the\n" +
			"`current_organization` URI behind every `--org` scope and `org members`,\n" +
			"the account's public `scheduling_url`, and the timezone that bare local\n" +
			"times would otherwise be guessed in. It is also the cheapest check that the\n" +
			"connection is live and which account it points at.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
