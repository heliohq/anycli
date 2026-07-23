package hunter

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newDomainSearchCmd wraps GET /domain-search: all email addresses Hunter knows
// for a domain (or a company name). 1 credit per 1-10 emails returned.
func (s *Service) newDomainSearchCmd(key string) *cobra.Command {
	var domain, company, department, seniority, requiredField, verificationStatus, typ string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "domain-search",
		Short: "Find email addresses for a domain (GET /domain-search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "domain", domain)
			setIf(q, "company", company)
			setIf(q, "department", department)
			setIf(q, "seniority", seniority)
			setIf(q, "required_field", requiredField)
			setIf(q, "verification_status", verificationStatus)
			setIf(q, "type", typ)
			if cmd.Flags().Changed("limit") {
				q.Set("limit", itoa(limit))
			}
			if cmd.Flags().Changed("offset") {
				q.Set("offset", itoa(offset))
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/domain-search", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain to search (e.g. stripe.com)")
	cmd.Flags().StringVar(&company, "company", "", "company name (alternative to --domain)")
	cmd.Flags().StringVar(&department, "department", "", "filter by department (comma-separated)")
	cmd.Flags().StringVar(&seniority, "seniority", "", "filter by seniority: junior|senior|executive")
	cmd.Flags().StringVar(&requiredField, "required-field", "", "only emails with these fields (comma-separated)")
	cmd.Flags().StringVar(&verificationStatus, "verification-status", "", "filter by verification status")
	cmd.Flags().StringVar(&typ, "type", "", "email type: personal|generic")
	cmd.Flags().IntVar(&limit, "limit", 0, "max emails to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}

// newEmailCountCmd wraps GET /email-count: how many emails Hunter has for a
// domain/company. Free of charge, no credential-independent counting cost.
func (s *Service) newEmailCountCmd(key string) *cobra.Command {
	var domain, company, typ string
	cmd := &cobra.Command{
		Use:   "email-count",
		Short: "Count known emails for a domain (GET /email-count)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "domain", domain)
			setIf(q, "company", company)
			setIf(q, "type", typ)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/email-count", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain to count (e.g. stripe.com)")
	cmd.Flags().StringVar(&company, "company", "", "company name (alternative to --domain)")
	cmd.Flags().StringVar(&typ, "type", "", "email type: personal|generic")
	return cmd
}

// newDomainFinderCmd wraps GET /domain-finder (beta): company name -> domain.
// Free of charge.
func (s *Service) newDomainFinderCmd(key string) *cobra.Command {
	var company string
	var limit int
	var perfectMatch bool
	cmd := &cobra.Command{
		Use:   "domain-finder",
		Short: "Find the domain for a company name (GET /domain-finder)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("company", company)
			if cmd.Flags().Changed("limit") {
				q.Set("limit", itoa(limit))
			}
			if perfectMatch {
				q.Set("perfect_match", "true")
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/domain-finder", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&company, "company", "", "company name (>= 3 chars)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max candidates (1-10, default 5)")
	cmd.Flags().BoolVar(&perfectMatch, "perfect-match", false, "only return an exact-match domain")
	_ = cmd.MarkFlagRequired("company")
	return cmd
}
