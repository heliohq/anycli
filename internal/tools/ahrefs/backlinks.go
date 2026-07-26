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

// The two backlink Long texts. They sit next to the shared builder because it
// is the builder that fixes the target-only, dateless row contract both
// describe.
const (
	longBacklinksList = "One row per individual LINK, so a site with many links from one\n" +
		"referring domain fills the page with near-duplicates — `refdomains`\n" +
		"answers \"who links to us\" for fewer units. --target is required; there\n" +
		"is no --date, this is the live index. Rank the useful ones with\n" +
		"`--order-by 'traffic:desc'` and cut the noise with `--where`, whose\n" +
		"expression is Ahrefs' own syntax passed through verbatim (for example\n" +
		"domain_rating_source>50)."
	longBacklinksBroken = "Links pointing at the target that no longer resolve — the `http_code`\n" +
		"column is why each is broken. Two jobs use this: reclaiming links to\n" +
		"pages that 404 on the target's own site, and finding dead links on other\n" +
		"sites worth replacing. --target is required and there is no --date;\n" +
		"`--order-by` and `--where` work as on `backlinks list`."
)

// newBacklinksCmd builds the `backlinks` group over Site Explorer's backlink
// endpoints: `list` (all-backlinks) and `broken` (broken-backlinks).
func (s *Service) newBacklinksCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "backlinks", Short: "Backlinks pointing at a target"}
	cmd.AddCommand(
		s.newBacklinksRowsCmd(token, "list", "Who links to a target (GET /site-explorer/all-backlinks)",
			longBacklinksList, "/site-explorer/all-backlinks", backlinksDefaultSelect),
		s.newBacklinksRowsCmd(token, "broken", "Broken/lost backlinks to a target (GET /site-explorer/broken-backlinks)",
			longBacklinksBroken, "/site-explorer/broken-backlinks", brokenDefaultSelect),
	)
	return cmd
}

// newBacklinksRowsCmd builds one target+rows backlink command. all-backlinks and
// broken-backlinks require select+target (no date) and accept the shared rows
// filter grammar plus target mode/protocol.
func (s *Service) newBacklinksRowsCmd(token, use, short, long, path, defaultSelect string) *cobra.Command {
	var target string
	var rf rowFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
