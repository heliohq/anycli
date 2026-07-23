package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMeCmd fetches the authenticated user (GET /v2/me) — identity check and the
// bundle's identity probe.
func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "me",
		Short:       "Fetch the authenticated user (GET /v2/me)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
