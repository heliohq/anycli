package dataforseo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newBacklinksCmd is the `backlinks` resource group: summary, list, referring
// domains, and anchors.
func (s *Service) newBacklinksCmd(credential string) *cobra.Command {
	bl := newGroupCmd("backlinks", "Backlink profile analysis")
	bl.AddCommand(
		s.newBacklinksSummaryCmd(credential),
		s.newBacklinksListCmd(credential),
		s.newBacklinksReferringDomainsCmd(credential),
		s.newBacklinksAnchorsCmd(credential),
	)
	return bl
}

// newBacklinksSummaryCmd returns aggregate backlink metrics for a target. The
// summary endpoint has no `limit` (it caps internal arrays via
// internal_list_limit), so no --limit flag is offered.
func (s *Service) newBacklinksSummaryCmd(credential string) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Aggregate backlink metrics for a domain or URL",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"target": target}
			return s.do(cmd.Context(), credential, http.MethodPost, "/backlinks/summary/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&target, "target", "", "domain, subdomain, or URL (required)")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

// newBacklinksListCmd lists individual backlinks pointing at a target.
func (s *Service) newBacklinksListCmd(credential string) *cobra.Command {
	var (
		target string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Individual backlinks pointing at a domain or URL",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"target": target}
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/backlinks/backlinks/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&target, "target", "", "domain, subdomain, or URL (required)")
	_ = cmd.MarkFlagRequired("target")
	cmd.Flags().IntVar(&limit, "limit", 0, "max backlinks (default 100, max 1000)")
	return cmd
}

// newBacklinksReferringDomainsCmd lists the domains linking to a target.
func (s *Service) newBacklinksReferringDomainsCmd(credential string) *cobra.Command {
	var (
		target string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "referring-domains",
		Short: "Domains referring to a target",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"target": target}
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/backlinks/referring_domains/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&target, "target", "", "domain, subdomain, or URL (required)")
	_ = cmd.MarkFlagRequired("target")
	cmd.Flags().IntVar(&limit, "limit", 0, "max referring domains (default 100, max 1000)")
	return cmd
}

// newBacklinksAnchorsCmd returns the anchor-text distribution for a target.
func (s *Service) newBacklinksAnchorsCmd(credential string) *cobra.Command {
	var (
		target string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "anchors",
		Short: "Anchor-text distribution for a target",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"target": target}
			if limit > 0 {
				task["limit"] = limit
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/backlinks/anchors/live", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&target, "target", "", "domain, subdomain, or URL (required)")
	_ = cmd.MarkFlagRequired("target")
	cmd.Flags().IntVar(&limit, "limit", 0, "max anchors (default 100, max 1000)")
	return cmd
}
