package posthog

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newWhoamiCmd prints the authenticated user and the resolved region host. The
// region host is written to stderr so stdout stays clean provider JSON.
func (s *Service) newWhoamiCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "whoami",
		Short:       "Show the authenticated user and resolved region host (GET /api/users/@me)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, host, err := s.self(cmd.Context(), token)
			if err != nil {
				return err
			}
			if err := s.emit(body); err != nil {
				return err
			}
			fmt.Fprintf(s.stderr(), "resolved region host: %s\n", host)
			return nil
		},
	}
}
