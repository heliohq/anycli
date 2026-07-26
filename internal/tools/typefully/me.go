package typefully

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMeCmd is `typefully me` — GET /v2/me. Resolves the authenticated account
// from the key (connect-time verify + "who am I").
func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get the authenticated Typefully account (GET /v2/me)",
		Long: "The only command that takes no --social-set, which makes it the cheap\n" +
			"credential check: a rejected key fails here before any draft is touched.\n" +
			"It identifies the account, not its social sets — those come from\n" +
			"`social-set list`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// scopedPath builds a social-set-scoped path: /social-sets/{id}{suffix}.
func scopedPath(socialSet, suffix string) string {
	return "/social-sets/" + socialSet + suffix
}

// addSocialSetFlag wires the required --social-set flag shared by scoped
// commands (a missing required flag is a cobra parse error -> exit 2).
func addSocialSetFlag(cmd *cobra.Command, socialSet *string) {
	cmd.Flags().StringVar(socialSet, "social-set", "", "social set id (from `social-set list`); required")
	_ = cmd.MarkFlagRequired("social-set")
}
