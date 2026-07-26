package snov

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAccountCmd groups account-level reads. `balance` reports remaining Snov
// credits and doubles as the connectivity / credential check (it exercises the
// full client_credentials exchange without consuming credits).
func (s *Service) newAccountCmd(creds clientCreds) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Account credits and status",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "balance",
		Short: "Report remaining Snov.io credits (free; also the connectivity check)",
		Long: "Exercises the full client-credentials exchange without spending anything,\n" +
			"so a failure here separates a bad or lapsed credential from a problem with\n" +
			"a specific search. Worth one call before a batch of finds or\n" +
			"verifications: those fail on an exhausted balance, and this is the only\n" +
			"place the remaining credits are visible.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.callV1(cmd.Context(), creds, http.MethodGet, "/v1/get-balance", url.Values{})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	})
	return cmd
}
