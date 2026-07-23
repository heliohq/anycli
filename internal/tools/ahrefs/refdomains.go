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
		Args:  cobra.NoArgs,
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
