package loops

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newWhoamiCmd verifies the API key and returns the team context
// (GET /v1/api-key → {success, teamName}). This is also the identity endpoint
// the Helio connect flow verifies against.
func (s *Service) newWhoamiCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Verify the API key and show team context (GET /v1/api-key)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/v1/api-key", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
