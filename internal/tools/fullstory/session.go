package fullstory

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newSessionCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "User session replays"}
	cmd.AddCommand(s.newSessionListCmd(key))
	return cmd
}

// newSessionListCmd wraps GET /v2/sessions — the primary "investigate this
// user" entry point: given a uid and/or email, return the most recent session
// replay URLs for that user (results[].{id, fs_url, created_time}).
func (s *Service) newSessionListCmd(key string) *cobra.Command {
	var uid, email string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a user's recent session replay URLs (GET /v2/sessions)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if uid == "" && email == "" {
				return &usageError{msg: "session list requires --uid or --email"}
			}
			q := url.Values{}
			if uid != "" {
				q.Set("uid", uid)
			}
			if email != "" {
				q.Set("email", email)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/v2/sessions", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&uid, "uid", "", "application-specific user id")
	cmd.Flags().StringVar(&email, "email", "", "user email")
	cmd.Flags().IntVar(&limit, "limit", 0, "max sessions to return (0 = provider default)")
	return cmd
}
