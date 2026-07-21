package ahrefs

import (
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// newDomainCmd builds the `domain` resource group. Its one leaf, `overview`,
// answers the #1 ask ("how strong is this domain?") by fanning out to the three
// scalar Site Explorer endpoints and merging them into one object.
func (s *Service) newDomainCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "domain", Short: "Domain-level Site Explorer metrics"}
	cmd.AddCommand(s.newDomainOverviewCmd(token))
	return cmd
}

// newDomainOverviewCmd merges GET /site-explorer/{domain-rating,backlinks-stats,
// metrics} for one target. All three require target+date; --cheap limits the
// call to domain-rating only (50 units instead of up to 150). The merged output
// is keyed by endpoint so an agent can read the source of each figure.
func (s *Service) newDomainOverviewCmd(token string) *cobra.Command {
	var target, date, country, mode, protocol string
	var cheap bool
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Domain Rating + backlink + traffic/keyword metrics for a target",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				return &usageError{msg: "ahrefs: --target is required"}
			}
			day := date
			if day == "" {
				day = time.Now().UTC().Format("2006-01-02")
			}

			merged := map[string]any{}

			drQuery := url.Values{}
			drQuery.Set("target", target)
			drQuery.Set("date", day)
			dr, err := s.call(cmd.Context(), token, http.MethodGet, "/site-explorer/domain-rating", drQuery, nil)
			if err != nil {
				return err
			}
			merged["domain_rating"] = rawJSON(dr)

			if !cheap {
				statsQuery := url.Values{}
				statsQuery.Set("target", target)
				statsQuery.Set("date", day)
				applyTargetMode(statsQuery, mode, protocol)
				stats, err := s.call(cmd.Context(), token, http.MethodGet, "/site-explorer/backlinks-stats", statsQuery, nil)
				if err != nil {
					return err
				}
				merged["backlinks_stats"] = rawJSON(stats)

				metricsQuery := url.Values{}
				metricsQuery.Set("target", target)
				metricsQuery.Set("date", day)
				if country != "" {
					metricsQuery.Set("country", country)
				}
				applyTargetMode(metricsQuery, mode, protocol)
				metrics, err := s.call(cmd.Context(), token, http.MethodGet, "/site-explorer/metrics", metricsQuery, nil)
				if err != nil {
					return err
				}
				merged["metrics"] = rawJSON(metrics)
			}

			return s.emitValue(merged)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "domain or URL to analyze (required)")
	cmd.Flags().StringVar(&date, "date", "", "report date YYYY-MM-DD (default: today UTC)")
	cmd.Flags().StringVar(&country, "country", "", "ISO country code for the metrics slice (optional)")
	cmd.Flags().StringVar(&mode, "mode", "", "target mode: exact|prefix|domain|subdomains")
	cmd.Flags().StringVar(&protocol, "protocol", "", "protocol: both|http|https")
	cmd.Flags().BoolVar(&cheap, "cheap", false, "domain-rating only (skip backlinks-stats + metrics)")
	return cmd
}

// applyTargetMode sets the shared mode/protocol query params when non-empty.
func applyTargetMode(q url.Values, mode, protocol string) {
	if mode != "" {
		q.Set("mode", mode)
	}
	if protocol != "" {
		q.Set("protocol", protocol)
	}
}
