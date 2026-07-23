package dataforseo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newDomainCmd is the `domain` resource group: organic overview, ranked
// keywords, and SERP competitors.
func (s *Service) newDomainCmd(credential string) *cobra.Command {
	domain := newGroupCmd("domain", "Domain and competitor research")
	domain.AddCommand(
		s.newDomainOverviewCmd(credential),
		s.newDomainRankedKeywordsCmd(credential),
		s.newDomainCompetitorsCmd(credential),
	)
	return domain
}

// newDomainOverviewCmd returns a domain's organic/paid visibility overview.
func (s *Service) newDomainOverviewCmd(credential string) *cobra.Command {
	var (
		target string
		tp     taskParams
	)
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Organic and paid visibility overview for a domain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"target": target}
			tp.apply(task)
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/domain_rank_overview/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&target, "target", "", "domain, e.g. example.com without protocol (required)")
	_ = cmd.MarkFlagRequired("target")
	registerLocationLang(cmd, &tp)
	return cmd
}

// newDomainRankedKeywordsCmd lists the keywords a domain ranks for.
func (s *Service) newDomainRankedKeywordsCmd(credential string) *cobra.Command {
	var (
		target string
		tp     taskParams
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "ranked-keywords",
		Short: "Keywords a domain currently ranks for",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"target": target}
			tp.apply(task)
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/ranked_keywords/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&target, "target", "", "domain, subdomain, or URL (required)")
	_ = cmd.MarkFlagRequired("target")
	cmd.Flags().IntVar(&limit, "limit", 0, "max keywords (default 100, max 1000)")
	registerLocationLang(cmd, &tp)
	return cmd
}

// newDomainCompetitorsCmd lists a domain's organic SERP competitors.
func (s *Service) newDomainCompetitorsCmd(credential string) *cobra.Command {
	var (
		target string
		tp     taskParams
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "competitors",
		Short: "Organic SERP competitors for a domain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"target": target}
			tp.apply(task)
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/dataforseo_labs/google/competitors_domain/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&target, "target", "", "domain, e.g. example.com without protocol (required)")
	_ = cmd.MarkFlagRequired("target")
	cmd.Flags().IntVar(&limit, "limit", 0, "max competitors (default 100, max 1000)")
	registerLocationLang(cmd, &tp)
	return cmd
}
