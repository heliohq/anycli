package semrush

import "github.com/spf13/cobra"

// newDomainCmd groups the domain-level SEO reports (organic/paid visibility,
// history, competitors, top pages). Costs run 10–40 API units per line — see
// each report's Semrush documentation.
func (s *Service) newDomainCmd(key string) *cobra.Command {
	group := newGroupCmd("domain", "Domain-level SEO reports")
	group.AddCommand(
		s.newReportCmd(key, reportSpec{
			use: "overview", short: "Domain rank + traffic overview (one or all databases)",
			typ: "domain_rank", allDBTyp: "domain_ranks", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "history", short: "Historical domain rank/traffic snapshots",
			typ: "domain_rank_history", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "organic", short: "Keywords a domain ranks for in organic search",
			typ: "domain_organic", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "paid", short: "Keywords a domain buys in paid search",
			typ: "domain_adwords", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "competitors", short: "Competing domains in organic (or --paid) search",
			typ: "domain_organic_organic", altTyp: "domain_adwords_adwords", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "pages", short: "A domain's top organic landing pages",
			typ: "domain_organic_unique", subject: "domain", argName: "<domain>",
		}),
	)
	return group
}

// newKeywordCmd groups the keyword-research reports (volume/CPC/difficulty,
// related/broad/questions, per-keyword SERP results).
func (s *Service) newKeywordCmd(key string) *cobra.Command {
	group := newGroupCmd("keyword", "Keyword-research reports")
	group.AddCommand(
		s.newReportCmd(key, reportSpec{
			use: "overview", short: "Keyword volume/CPC/competition (one or all databases)",
			typ: "phrase_this", allDBTyp: "phrase_all", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "batch", short: "Overview for several keywords at once",
			typ: "phrase_these", subject: "phrase", argName: "<phrase>...", joinArg: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "related", short: "Keywords semantically related to a phrase",
			typ: "phrase_related", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "broad", short: "Broad-match keywords containing a phrase",
			typ: "phrase_fullsearch", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "questions", short: "Question keywords containing a phrase",
			typ: "phrase_questions", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "difficulty", short: "Keyword Difficulty Index for several keywords",
			typ: "phrase_kdi", subject: "phrase", argName: "<phrase>...", joinArg: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "organic-results", short: "Domains ranking organically for a keyword",
			typ: "phrase_organic", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "paid-results", short: "Domains advertising on a keyword",
			typ: "phrase_adwords", subject: "phrase", argName: "<phrase>",
		}),
	)
	return group
}

// newURLCmd groups the per-URL reports (keywords a specific page ranks/buys).
func (s *Service) newURLCmd(key string) *cobra.Command {
	group := newGroupCmd("url", "Per-URL keyword reports")
	group.AddCommand(
		s.newReportCmd(key, reportSpec{
			use: "organic", short: "Organic keywords for a specific URL",
			typ: "url_organic", subject: "url", argName: "<url>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "paid", short: "Paid keywords for a specific URL",
			typ: "url_adwords", subject: "url", argName: "<url>",
		}),
	)
	return group
}

// newBacklinksCmd groups the backlinks reports. These live under the
// /analytics/v1/ base, are global (no database), and take --target-type
// (root_domain|domain|url).
func (s *Service) newBacklinksCmd(key string) *cobra.Command {
	group := newGroupCmd("backlinks", "Backlink-profile reports")
	group.AddCommand(
		s.newReportCmd(key, reportSpec{
			use: "overview", short: "Backlink profile summary for a target",
			typ: "backlinks_overview", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "list", short: "Individual backlinks pointing at a target",
			typ: "backlinks", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "refdomains", short: "Referring domains linking to a target",
			typ: "backlinks_refdomains", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "anchors", short: "Anchor texts used in backlinks to a target",
			typ: "backlinks_anchors", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "pages", short: "Pages of a target that receive backlinks",
			typ: "backlinks_pages", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "competitors", short: "Domains with a similar backlink profile",
			typ: "backlinks_competitors", subject: "target", argName: "<target>", backlinks: true,
		}),
	)
	return group
}
