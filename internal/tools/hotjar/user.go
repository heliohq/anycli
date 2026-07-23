package hotjar

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// newUserCmd groups the GDPR/ops user-lookup surface.
func (s *Service) newUserCmd(creds clientCreds) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Look up a data subject's captured data (GDPR/ops)",
	}
	cmd.AddCommand(s.newUserLookupCmd(creds))
	return cmd
}

// newUserLookupCmd looks up the data captured for a data subject by email.
//
// SAFETY (Divergence 4): Hotjar's user-lookup endpoint doubles as its deletion
// endpoint — the same POST with delete_all_hits:true silently purges the
// subject's data. This command therefore ALWAYS sends delete_all_hits:false and
// exposes no flag that can flip it, so the destructive mode is structurally
// unreachable from the toolset. Do not add a delete flag here; a separate,
// human-gated deletion tool would be its own reviewed change.
func (s *Service) newUserLookupCmd(creds clientCreds) *cobra.Command {
	var org, email string
	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Find a data subject's captured data by email (read-only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{
				"data_subject_email": email,
				// Pinned false — lookup must never delete. See the SAFETY note.
				"delete_all_hits": false,
			}
			body, err := s.post(cmd.Context(), creds,
				fmt.Sprintf("/v1/organizations/%s/user-lookup", url.PathEscape(org)), payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Hotjar organization id (required)")
	cmd.Flags().StringVar(&email, "email", "", "data subject email to look up (required)")
	_ = cmd.MarkFlagRequired("org")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
