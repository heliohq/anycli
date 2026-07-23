package fullstory

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMeCmd wraps GET /me — FullStory's documented "test your key" endpoint. It
// returns the key's role (USER/ARCHITECT/ADMIN), doubling as a connectivity and
// permission-level check for the AI teammate.
func (s *Service) newMeCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "me",
		Short:       "Show the API key's role and verify connectivity (GET /me)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
