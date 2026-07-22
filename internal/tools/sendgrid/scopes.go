package sendgrid

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newScopesCmd exposes GET /v3/scopes: verify the key and list the scopes it
// carries. A least-privilege mail.send-only key returns 403 here (valid but not
// scope-readable); a Full Access key returns {"scopes":[...]}.
func (s *Service) newScopesCmd(token string, region *string) *cobra.Command {
	return &cobra.Command{
		Use:   "scopes",
		Short: "List the API key's granted scopes (GET /v3/scopes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/scopes", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
