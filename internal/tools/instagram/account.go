package instagram

import (
	"net/url"

	"github.com/spf13/cobra"
)

// accountFields are the default /me fields for `account get`. The connection
// token is scoped to one Instagram professional account, so /me is that
// account (no selector).
const accountFields = "user_id,username,name,biography,followers_count,follows_count,media_count,profile_picture_url"

func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get the connected Instagram professional account (GET /me)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("fields", firstNonEmpty(fields, accountFields))
			body, err := s.get(cmd.Context(), token, "/me", q)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated field list (default: profile summary)")
	return cmd
}

// firstNonEmpty returns override when set, else fallback.
func firstNonEmpty(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}
