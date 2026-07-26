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
		Use:   "search --data <json|@file>",
		Short: "Run an Advanced Filtering query (POST /data/search/)",
		Long: "The only way to find records by anything other than an id — the `list`\n" +
			"verbs have no filters whatsoever. --data is forwarded verbatim, so the\n" +
			"whole Advanced Filtering DSL is available: `object_type` to pick the\n" +
			"record kind, `field_condition` over regular and custom fields,\n" +
			"`has_related` to reach across leads, contacts and opportunities, plus\n" +
			"`_limit` and `cursor` for paging.\n" +
			"\n" +
			"Queries nest several levels deep, so @file.json is usually easier to get\n" +
			"right than an inline string, and the same file can be re-run as a saved\n" +
			"report. The response is Close's own search envelope, not the plain list\n" +
			"envelope the `list` verbs return.",
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
		Use:   "me",
		Short: "Show the authenticated user and their organizations",
		Long: "The authenticated user plus the organizations this token can act on. Worth\n" +
			"reading first when a lead or contact cannot be found: a token reaches only\n" +
			"its own organizations, so the usual cause is the wrong organization rather\n" +
			"than a deleted record. The organization id it returns is also what some\n" +
			"`search` queries need.",
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
