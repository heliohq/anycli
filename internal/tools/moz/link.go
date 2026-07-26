package moz

import (
	"github.com/spf13/cobra"
)

func (s *Service) newLinkCmd(token string) *cobra.Command {
	cmd := newGroupCmd("link", "Backlinks, linking root domains, and anchor text")
	cmd.AddCommand(
		s.newLinkListCmd(token),
		s.newLinkDomainsCmd(token),
		s.newLinkAnchorsCmd(token),
	)
	return cmd
}

// longLinkList, longLinkDomains and longLinkAnchors are the three link Longs.
// They live next to the shared builder because it is the builder that fixes the
// target_query shape and the per-row quota cost all three describe; what
// differs is what one row means in each.
const (
	longLinkList = "One row per inbound LINK, so a single site that links a thousand times\n" +
		"consumes a thousand rows of quota — when the question is who links rather\n" +
		"than how often, `link domains` answers it far more cheaply. Each row\n" +
		"carries the source page, its anchor text and its authority. --limit\n" +
		"defaults to 25."

	longLinkDomains = "One row per linking ROOT DOMAIN rather than per link, which makes this\n" +
		"both the cheap read and the honest measure of link diversity: a hundred\n" +
		"links from one site collapse to one row here. --limit defaults to 25."

	longLinkAnchors = "Anchor phrases aggregated across the target's inbound links, one row per\n" +
		"phrase with its link and domain counts. This is where an over-optimised\n" +
		"or spam-looking anchor profile becomes visible, which the raw per-link\n" +
		"output of `link list` buries. --limit defaults to 25."
)

// newLinkListCmd lists inbound links to a target (data.site.link.list).
func (s *Service) newLinkListCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "list",
		short:  "Inbound links pointing at a target",
		long:   longLinkList,
		method: "data.site.link.list",
	})
}

// newLinkDomainsCmd lists linking root domains for a target
// (data.site.linking-domain.list).
func (s *Service) newLinkDomainsCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "domains",
		short:  "Linking root domains for a target",
		long:   longLinkDomains,
		method: "data.site.linking-domain.list",
	})
}

// newLinkAnchorsCmd lists the anchor-text profile for a target
// (data.site.anchor-text.list).
func (s *Service) newLinkAnchorsCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "anchors",
		short:  "Anchor-text profile for a target",
		long:   longLinkAnchors,
		method: "data.site.anchor-text.list",
	})
}

// targetListSpec parameterizes the target_query-scoped list commands, which
// share the same request shape ({target_query:{query,scope?}, limit}) and
// differ only in method name and help text.
type targetListSpec struct {
	use    string
	short  string
	long   string
	method string
}

// newTargetListCmd builds one target_query-scoped list command from a spec.
func (s *Service) newTargetListCmd(token string, spec targetListSpec) *cobra.Command {
	var site, scope string
	var limit int
	cmd := &cobra.Command{
		Use:         spec.use,
		Short:       spec.short,
		Long:        spec.long,
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if site == "" {
				return &usageError{msg: "moz: --site is required"}
			}
			if err := checkScope(scope); err != nil {
				return err
			}
			data := map[string]any{"target_query": siteQuery(site, scope), "limit": limit}
			result, err := s.call(cmd.Context(), token, spec.method, data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "target URL/domain")
	cmd.Flags().StringVar(&scope, "scope", "", "query scope: page|subdomain|root_domain (default: API default)")
	cmd.Flags().IntVar(&limit, "limit", 25, "max rows to return (each returned row bills quota)")
	return cmd
}
