package wise

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newProfileListCmd lists the token's personal + business profiles. This is
// also the identity / "who am I" call: every profile-scoped verb needs a
// profileId resolved from here.
func (s *Service) newProfileListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List the token's profiles (GET /v1/profiles)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/profiles", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
