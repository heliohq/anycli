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
		Long: "Answers 403 for a restricted key — one minted with, say, mail.send only\n" +
			"cannot read its own scope list — so a 403 here confirms the key is live\n" +
			"and narrow, not revoked. A Full Access key returns the `scopes` array.\n" +
			"Read it once before a multi-step task to learn which surfaces are\n" +
			"reachable instead of discovering it from a mid-task failure.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/scopes", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
