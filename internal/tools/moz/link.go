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

// newLinkListCmd lists inbound links to a target (data.site.link.list).
func (s *Service) newLinkListCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "list",
		short:  "Inbound links pointing at a target",
		method: "data.site.link.list",
	})
}

// newLinkDomainsCmd lists linking root domains for a target
// (data.site.linking-domain.list).
func (s *Service) newLinkDomainsCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "domains",
		short:  "Linking root domains for a target",
		method: "data.site.linking-domain.list",
	})
}

// newLinkAnchorsCmd lists the anchor-text profile for a target
// (data.site.anchor-text.list).
func (s *Service) newLinkAnchorsCmd(token string) *cobra.Command {
	return s.newTargetListCmd(token, targetListSpec{
		use:    "anchors",
		short:  "Anchor-text profile for a target",
		method: "data.site.anchor-text.list",
	})
}

// targetListSpec parameterizes the target_query-scoped list commands, which
// share the same request shape ({target_query:{query,scope?}, limit}) and
// differ only in method name and help text.
type targetListSpec struct {
	use    string
	short  string
	method string
}

// newTargetListCmd builds one target_query-scoped list command from a spec.
func (s *Service) newTargetListCmd(token string, spec targetListSpec) *cobra.Command {
	var site, scope string
	var limit int
	cmd := &cobra.Command{
		Use:   spec.use,
		Short: spec.short,
		Args:  cobra.NoArgs,
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
