package typeform

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newMeCmd is `me` (GET /me): the authenticated account's identity. The
// documented accounts:read response carries exactly alias, email, and language
// (https://www.typeform.com/developers/get-started/hands-on/) — there is no
// user_id field. Output JSON.
func (s *Service) newMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Retrieve the authenticated Typeform account (GET /me)",
		Long: "Returns exactly `alias`, `email` and `language` for the connected account.\n" +
			"There is no user id in this response and no command in the tool takes one,\n" +
			"so ownership questions have to be answered from a form's own workspace\n" +
			"rather than from an account identifier.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}
