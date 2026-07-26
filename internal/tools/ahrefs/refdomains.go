package ahrefs

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

const refdomainsDefaultSelect = "domain,domain_rating,links_to_target,dofollow_links,first_seen,last_seen,traffic_domain"

// newRefdomainsCmd builds the `refdomains` command over Site Explorer's
// referring-domains view (cheaper than raw backlinks for "who links to us").
// GET /site-explorer/refdomains requires select+target (no date).
func (s *Service) newRefdomainsCmd(token string) *cobra.Command {
	var target string
	var rf rowFlags
	cmd := &cobra.Command{
		Use:   "refdomains",
		Short: "Referring domains for a target (GET /site-explorer/refdomains)",
		Long: "One row per linking DOMAIN rather than per link, which is both the honest\n" +
			"measure of link diversity and materially cheaper than `backlinks list` for\n" +
			"the same question. --target is required and there is no --date. The\n" +
			"default fields carry `links_to_target` and `dofollow_links`, so the\n" +
			"per-domain link count is available without dropping down to individual\n" +
			"backlinks.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				return &usageError{msg: "ahrefs: --target is required"}
			}
			q := url.Values{}
			q.Set("target", target)
			rf.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/site-explorer/refdomains", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "domain or URL to analyze (required)")
	registerRowFlags(cmd, &rf, refdomainsDefaultSelect, true)
	return cmd
}
