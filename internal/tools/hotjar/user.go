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
		Long: "POSTs to the organization's user-lookup endpoint, which is also Hotjar's\n" +
			"data-DELETION endpoint — the same call with `delete_all_hits` set true\n" +
			"purges everything Hotjar holds on the subject. This command pins that flag\n" +
			"to false and exposes no way to flip it, so it can only read; a genuine\n" +
			"erasure request has to be done in Hotjar's own UI.\n" +
			"\n" +
			"`--org` is an organization id, not the `--site` id the survey commands take.\n" +
			"Matching is on the exact `--email`; there is no lookup by user id, partial\n" +
			"address or any other attribute. Needs at least the Observe Scale plan.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
