package apollo

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newOrgCmd builds the `org` group: find target accounts (search) and pull
// firmographics from a domain (enrich / bulk-enrich).
func (s *Service) newOrgCmd(token string) *cobra.Command {
	cmd := newGroupCmd("org", "Find and enrich companies (accounts)")
	cmd.AddCommand(
		s.newOrgSearchCmd(token),
		s.newOrgEnrichCmd(token),
		s.newOrgBulkEnrichCmd(token),
	)
	return cmd
}

// newOrgSearchCmd wraps POST /mixed_companies/search.
func (s *Service) newOrgSearchCmd(token string) *cobra.Command {
	var body string
	var industries, locations []string
	var employeesMin, employeesMax, page, perPage int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search for companies by industry/location/size (POST /mixed_companies/search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStrSlice(b, "q_organization_keyword_tags", industries)
			setStrSlice(b, "organization_locations", locations)
			if employeesMin > 0 || employeesMax > 0 {
				lo, hi := employeesMin, employeesMax
				if lo == 0 {
					lo = 1
				}
				if hi == 0 {
					hi = 1000000
				}
				b["organization_num_employees_ranges"] = []string{strconv.Itoa(lo) + "," + strconv.Itoa(hi)}
			}
			applyPageBody(b, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/mixed_companies/search", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&industries, "industry", nil, "industry/keyword tag filter (repeatable)")
	cmd.Flags().StringArrayVar(&locations, "location", nil, "company location filter (repeatable)")
	cmd.Flags().IntVar(&employeesMin, "employees-min", 0, "minimum employee count")
	cmd.Flags().IntVar(&employeesMax, "employees-max", 0, "maximum employee count")
	registerPageFlags(cmd, &page, &perPage)
	registerBodyFlag(cmd, &body)
	return cmd
}

// newOrgEnrichCmd wraps GET /organizations/enrich?domain=…
func (s *Service) newOrgEnrichCmd(token string) *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "enrich",
		Short: "Enrich a company by domain (GET /organizations/enrich)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("domain", domain)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/organizations/enrich", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "company domain (no www./@)")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

// newOrgBulkEnrichCmd wraps POST /organizations/bulk_enrich. Domains are passed
// as a repeatable --domain flag (Apollo `domains`).
func (s *Service) newOrgBulkEnrichCmd(token string) *cobra.Command {
	var body string
	var domains []string
	cmd := &cobra.Command{
		Use:   "bulk-enrich",
		Short: "Enrich multiple companies by domain (POST /organizations/bulk_enrich)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStrSlice(b, "domains", domains)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/organizations/bulk_enrich", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&domains, "domain", nil, "company domain (repeatable)")
	registerBodyFlag(cmd, &body)
	return cmd
}
