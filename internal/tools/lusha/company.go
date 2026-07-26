package lusha

import (
	"net/http"

	"github.com/spf13/cobra"
)

// companyRevealFields are the only values Lusha's company reveal selector
// accepts (V3 companies/enrich reveal enum) — firmographic expansions, NOT
// emails/phones (companies expose no contact PII to reveal).
var companyRevealFields = map[string]bool{
	"employeesByDepartment": true,
	"employeesByLocation":   true,
	"employeesBySeniority":  true,
	"competitors":           true,
	"intent":                true,
}

func (s *Service) newCompanyCmd(key string) *cobra.Command {
	cmd := newGroupCmd("company", "Companies (enrich by identifier, prospect by filter, reveal by id)")
	cmd.AddCommand(
		s.newCompanyEnrichCmd(key),
		s.newCompanySearchCmd(key),
		s.newCompanyRevealCmd(key),
	)
	return cmd
}

// newCompanyEnrichCmd is the one-shot known-identifier path: domain / name →
// firmographics via POST /companies/search-and-enrich. It has NO reveal
// selector — the request schema is {companies[], options} only; firmographics
// come back by default. Charged per successful result (reveal_company action).
func (s *Service) newCompanyEnrichCmd(key string) *cobra.Command {
	var domain, name string
	cmd := &cobra.Command{
		Use:   "enrich",
		Short: "Enrich a known company by domain or name (POST /companies/search-and-enrich)",
		Long: "Takes --domain or --name and returns firmographics. There is NO --reveal\n" +
			"flag here: the request schema has no reveal field, unlike the contact\n" +
			"verbs, so base firmographics come back by default and the expansions live\n" +
			"only on `company reveal`. Billed per successful result on the same\n" +
			"reveal_company meter `company reveal` uses — companies expose no\n" +
			"per-datapoint contact data to meter separately.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			item := map[string]any{}
			addIfSet(item, "domain", domain)
			addIfSet(item, "name", name)
			if len(item) == 0 {
				return &usageError{msg: "provide at least one identifier: --domain or --name"}
			}
			body := map[string]any{"companies": []any{item}}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/companies/search-and-enrich", body)
			if err != nil {
				return err
			}
			return s.emitRevealEnvelope(resp)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "company domain (e.g. lusha.com)")
	cmd.Flags().StringVar(&name, "name", "", "company name")
	return cmd
}

// newCompanySearchCmd is company-level ICP prospecting → company ids + a
// request id (name-only preview). Same nested filter DSL as contact search,
// passed as raw JSON. Charged api_search per result.
func (s *Service) newCompanySearchCmd(key string) *cobra.Command {
	var filtersJSON string
	var page, size int
	var includePartial bool
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Prospect net-new companies by filter (POST /companies/prospecting)",
		Long: "Returns name-only PREVIEWS carrying Lusha company ids, to be fed into\n" +
			"`company reveal`. --filters is the same raw prospecting object as `contact\n" +
			"search`, but only its `companies` branch is read here — include fields such\n" +
			"as sizes (`[{min,max}]`), revenues, industries, technologies, countries,\n" +
			"names and domains. Billed per result returned. --page is 0-based; keep\n" +
			"paging while `meta.has_more` is true.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filters, err := decodeFiltersFlag(filtersJSON)
			if err != nil {
				return err
			}
			body := map[string]any{
				"pagination": map[string]any{"page": page, "size": size},
				"filters":    filters,
			}
			if cmd.Flags().Changed("include-partial") {
				body["options"] = map[string]any{"includePartialProfiles": includePartial}
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/companies/prospecting", body)
			if err != nil {
				return err
			}
			return s.emitSearchEnvelope(resp)
		},
	}
	registerProspectingFlags(cmd, &filtersJSON, &page, &size, &includePartial)
	return cmd
}

// newCompanyRevealCmd is the reveal step for company prospecting: up to 100
// company ids (from a search result) → full firmographics via
// POST /companies/enrich. The optional reveal selector is the
// firmographic-expansion enum (NOT emails/phones). Charged per successful
// company result.
func (s *Service) newCompanyRevealCmd(key string) *cobra.Command {
	var ids []string
	var reveal string
	cmd := &cobra.Command{
		Use:   "reveal",
		Short: "Reveal companies by Lusha id (POST /companies/enrich)",
		Long: "Takes 1 to 100 --id values from a `company search`. --reveal here is a\n" +
			"FIRMOGRAPHIC-EXPANSION enum — employeesByDepartment, employeesByLocation,\n" +
			"employeesBySeniority, competitors, intent — and NOT the emails/phones\n" +
			"selector the contact verbs take; passing `emails` is rejected as a usage\n" +
			"error. Omit it for base firmographics only. Billed per successful company\n" +
			"result rather than per revealed datapoint.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateIDs(ids); err != nil {
				return err
			}
			body := map[string]any{"ids": ids}
			revealed, err := revealValues(reveal, companyRevealFields)
			if err != nil {
				return err
			}
			if revealed != nil {
				body["reveal"] = revealed
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/companies/enrich", body)
			if err != nil {
				return err
			}
			return s.emitRevealEnvelope(resp)
		},
	}
	cmd.Flags().StringArrayVar(&ids, "id", nil, "Lusha company id to reveal (repeatable, 1-100)")
	cmd.Flags().StringVar(&reveal, "reveal", "", "comma-separated firmographic expansions: employeesByDepartment,employeesByLocation,employeesBySeniority,competitors,intent (omit = base firmographics)")
	return cmd
}
