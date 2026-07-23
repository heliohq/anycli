package wise

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newActivityListCmd reads the unified, human-readable account activity feed.
// GET /v1/profiles/{profileId}/activities?status=&since=&until=&size=&nextCursor=
// Wise returns {"cursor": ..., "activities": [...]}; the response passes
// through verbatim so the AI can page with the returned cursor.
func (s *Service) newActivityListCmd(token string) *cobra.Command {
	var profile, status, since, until, nextCursor string
	var size int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List account activity (GET /v1/profiles/{id}/activities)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				return &usageError{msg: "wise activity list: --profile is required"}
			}
			q := url.Values{}
			if status != "" {
				q.Set("status", status)
			}
			if since != "" {
				q.Set("since", since)
			}
			if until != "" {
				q.Set("until", until)
			}
			if cmd.Flags().Changed("size") {
				q.Set("size", intToString(size))
			}
			if nextCursor != "" {
				q.Set("nextCursor", nextCursor)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/v1/profiles/"+url.PathEscape(profile)+"/activities", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "profile id (required)")
	cmd.Flags().StringVar(&status, "status", "", "activity status filter")
	cmd.Flags().StringVar(&since, "since", "", "ISO-8601 lower bound")
	cmd.Flags().StringVar(&until, "until", "", "ISO-8601 upper bound")
	cmd.Flags().IntVar(&size, "size", 0, "page size (1-100)")
	cmd.Flags().StringVar(&nextCursor, "next-cursor", "", "cursor from a previous page's response")
	return cmd
}
