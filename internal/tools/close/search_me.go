package close

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newSearchCmd runs Close's Advanced Filtering API: POST /data/search/ with a
// caller-supplied query body (JSON literal or @file). The body is forwarded
// verbatim, so the full query DSL — object_type, field_condition, has_related,
// cursor, _limit — is available without a bespoke flag surface.
func (s *Service) newSearchCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "search --data <json|@file>",
		Short:       "Run an Advanced Filtering query (POST /data/search/)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := readData("data", data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/data/search/", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "Advanced Filtering query as JSON (or @file.json)")
	return cmd
}

// newMeCmd fetches the authenticated user (GET /me/): id, name, email, and the
// organizations the token can act on. Also the identity/verify endpoint.
func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "me",
		Short:       "Show the authenticated user and their organizations",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me/", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
