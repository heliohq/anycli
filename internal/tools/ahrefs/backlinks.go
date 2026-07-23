package ahrefs

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// Curated default selects keep unit cost low: Ahrefs bills per (rows × fields),
// so each rows command requests only the columns an agent actually reads.
const (
	backlinksDefaultSelect = "url_from,url_to,anchor,domain_rating_source,traffic,first_seen,last_seen,is_dofollow"
	brokenDefaultSelect    = "url_from,url_to,anchor,domain_rating_source,http_code,first_seen,last_seen"
)

// newBacklinksCmd builds the `backlinks` group over Site Explorer's backlink
// endpoints: `list` (all-backlinks) and `broken` (broken-backlinks).
func (s *Service) newBacklinksCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "backlinks", Short: "Backlinks pointing at a target"}
	cmd.AddCommand(
		s.newBacklinksRowsCmd(token, "list", "Who links to a target (GET /site-explorer/all-backlinks)",
			"/site-explorer/all-backlinks", backlinksDefaultSelect),
		s.newBacklinksRowsCmd(token, "broken", "Broken/lost backlinks to a target (GET /site-explorer/broken-backlinks)",
			"/site-explorer/broken-backlinks", brokenDefaultSelect),
	)
	return cmd
}

// newBacklinksRowsCmd builds one target+rows backlink command. all-backlinks and
// broken-backlinks require select+target (no date) and accept the shared rows
// filter grammar plus target mode/protocol.
func (s *Service) newBacklinksRowsCmd(token, use, short, path, defaultSelect string) *cobra.Command {
	var target string
	var rf rowFlags
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				return &usageError{msg: "ahrefs: --target is required"}
			}
			q := url.Values{}
			q.Set("target", target)
			rf.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "domain or URL to analyze (required)")
	registerRowFlags(cmd, &rf, defaultSelect, true)
	return cmd
}
