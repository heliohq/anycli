package pandadoc

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// newWhoamiCmd fetches the authenticated member (GET /members/current) — the
// identity/whoami endpoint.
func (s *Service) newWhoamiCmd(authz string) *cobra.Command {
	return &cobra.Command{
		Use:         "whoami",
		Short:       "Show the authenticated PandaDoc member",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), authz, http.MethodGet, "/members/current", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			var m struct {
				UserID string `json:"user_id"`
				Email  string `json:"email"`
			}
			if err := json.Unmarshal(body, &m); err != nil {
				return s.emitJSON(body)
			}
			fmt.Fprintf(s.stdout(), "%s\t%s\n", m.Email, m.UserID)
			return nil
		},
	}
}
