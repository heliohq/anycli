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
		Long: "Costs one search credit per 1-10 addresses returned, so `--limit 10` is\n" +
			"one credit and `--limit 100` is ten — the most expensive command here.\n" +
			"Narrow before paying: `--department`, `--seniority`\n" +
			"(`junior|senior|executive`), `--type personal` to drop role addresses like\n" +
			"info@, and `--verification-status`. Continue with `--offset`. Each result\n" +
			"carries a confidence `score` and the `sources` Hunter saw it in. When only\n" +
			"the size of a domain matters, `email-count` answers for free.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "Free, and returns no addresses — only counts: the total Hunter holds for\n" +
			"the domain, split into `personal_emails` and `generic_emails`, plus a\n" +
			"department breakdown. It is the probe to run before spending credits on\n" +
			"`domain-search`, and the fast way to tell a domain Hunter knows nothing\n" +
			"about from one it holds hundreds of addresses for.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "Free, and marked beta by Hunter. Turns a company NAME into candidate\n" +
			"domains, which is the missing first step for the rest of the tool —\n" +
			"`domain-search`, `email-finder` and `enrich company` all want a domain.\n" +
			"`--company` needs at least 3 characters. `--limit` is 1-10, default 5,\n" +
			"and candidates come back ranked; `--perfect-match` returns only an exact\n" +
			"name match, or nothing at all.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
