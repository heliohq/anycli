package hunter

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newEmailVerifierCmd wraps GET /email-verifier: deliverability check for one
// address. Verification can take ~20s; if it is still running Hunter replies
// HTTP 202 (a success passthrough here — the body's data.status tells the agent
// to re-poll the same command, which costs one request). No client-side polling
// loop: the agent decides when to re-poll.
func (s *Service) newEmailVerifierCmd(key string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:         "email-verifier",
		Short:       "Verify an email address is deliverable (GET /email-verifier)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("email", email)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/email-verifier", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to verify")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
