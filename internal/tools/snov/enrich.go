package snov

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newEnrichCmd groups enrichment reads. `by-email` resolves a person profile
// (name, company, jobs, socials) from a known email address.
func (s *Service) newEnrichCmd(creds clientCreds) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enrich",
		Short: "Enrich a person or company",
	}
	cmd.AddCommand(s.newEnrichByEmailCmd(creds))
	return cmd
}

func (s *Service) newEnrichByEmailCmd(creds clientCreds) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:         "by-email",
		Short:       "Enrich a person profile from a known email (consumes credits)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := url.Values{}
			params.Set("email", email)
			body, err := s.callV1(cmd.Context(), creds, http.MethodPost, "/v1/get-profile-by-email", params)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to enrich (required)")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
