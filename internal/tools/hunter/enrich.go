package hunter

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newEnrichCmd groups the three enrichment endpoints under one verb:
//
//	enrich person   GET /people/find    (by --email or --linkedin-handle)
//	enrich company  GET /companies/find (by --domain)
//	enrich combined GET /combined/find  (by --email; person + company)
//
// Each returns 404 when Hunter has no record — surfaced as a plain error.
func (s *Service) newEnrichCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "enrich", Short: "Enrich a person or company (people/companies/combined find)"}
	cmd.AddCommand(
		s.newEnrichPersonCmd(key),
		s.newEnrichCompanyCmd(key),
		s.newEnrichCombinedCmd(key),
	)
	return cmd
}

func (s *Service) newEnrichPersonCmd(key string) *cobra.Command {
	var email, linkedinHandle string
	cmd := &cobra.Command{
		Use:         "person",
		Short:       "Enrich a person (GET /people/find)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "email", email)
			setIf(q, "linkedin_handle", linkedinHandle)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/people/find", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "person's email address")
	cmd.Flags().StringVar(&linkedinHandle, "linkedin-handle", "", "LinkedIn handle (alternative to --email)")
	return cmd
}

func (s *Service) newEnrichCompanyCmd(key string) *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:         "company",
		Short:       "Enrich a company (GET /companies/find)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("domain", domain)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/companies/find", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "company domain (e.g. stripe.com)")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func (s *Service) newEnrichCombinedCmd(key string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:         "combined",
		Short:       "Enrich a person and their company (GET /combined/find)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("email", email)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/combined/find", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "person's email address")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
