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
			long: "Returns a single row of Semrush rank, organic and paid keyword counts,\n" +
				"and estimated traffic and traffic cost for the `--database` region.\n" +
				"`--all-databases` switches to the aggregated form: one row per regional\n" +
				"database where the domain has any visibility, with `database` omitted\n" +
				"from the envelope. One line of cost makes this the report to run before\n" +
				"paging `domain organic` or `domain pages`.",
			typ: "domain_rank", allDBTyp: "domain_ranks", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "history", short: "Historical domain rank/traffic snapshots",
			long: "Returns one row per historical snapshot of the same rank and traffic\n" +
				"figures `domain overview` reports, for the `--database` region, so\n" +
				"`--limit` (default 10) counts snapshots rather than keywords.\n" +
				"`--date` pins a single snapshot as YYYYMMDD (`--date 20260115`).",
			typ: "domain_rank_history", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "organic", short: "Keywords a domain ranks for in organic search",
			long: "Returns one row per keyword the domain ranks for in the `--database`\n" +
				"region, with position, search volume, CPC, traffic share and the ranking\n" +
				"URL. This is the result set that grows fastest and every row is billed,\n" +
				"so page with `--limit` plus `--offset` instead of one large pull.\n" +
				"`--positions new|lost|rise|fall` narrows to keywords whose ranking moved.",
			typ: "domain_organic", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "paid", short: "Keywords a domain buys in paid search",
			long: "Returns one row per keyword the domain bids on in the `--database`\n" +
				"region, with the ad copy, ad position and estimated paid traffic and\n" +
				"cost. A domain that has never advertised in that region comes back as\n" +
				"`row_count` 0 with a `note` — an answer, not a failure.",
			typ: "domain_adwords", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "competitors", short: "Competing domains in organic (or --paid) search",
			long: "Ranks domains by organic keyword overlap with the subject domain, one\n" +
				"row per competitor with its common-keyword count and competition level.\n" +
				"`--paid` switches to the advertising-competition variant, which is a\n" +
				"genuinely different set — a domain that competes for organic rankings\n" +
				"often bids on nothing at all.",
			typ: "domain_organic_organic", altTyp: "domain_adwords_adwords", subject: "domain", argName: "<domain>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "pages", short: "A domain's top organic landing pages",
			long: "Returns the domain's landing pages with the organic traffic and keyword\n" +
				"count Semrush attributes to each, one row per URL, for the `--database`\n" +
				"region. Far fewer rows than `domain organic` for the same domain, and\n" +
				"the cheap way to pick a page worth drilling into with `url organic`.",
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
			long: "Returns one row of search volume, CPC, competitive density and total\n" +
				"result count for the phrase in the `--database` region.\n" +
				"`--all-databases` switches to the aggregated form — one row per regional\n" +
				"database — and omits `database` from the envelope. For several phrases\n" +
				"use `keyword batch`, which costs the same per line in a single call.",
			typ: "phrase_this", allDBTyp: "phrase_all", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "batch", short: "Overview for several keywords at once",
			long: "Takes the phrases as separate positional arguments and joins them with\n" +
				"`;` into one request, returning one row each with the columns of\n" +
				"`keyword overview`. `--all-databases` does not apply to this form, so\n" +
				"the whole batch is priced in the single `--database` region.",
			typ: "phrase_these", subject: "phrase", argName: "<phrase>...", joinArg: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "related", short: "Keywords semantically related to a phrase",
			long: "Returns semantically related phrases with their own volume and CPC.\n" +
				"Semrush prices this report at 40 units per line, four times a domain or\n" +
				"keyword overview, so raise `--limit` (default 10) deliberately. When the\n" +
				"requirement is phrases literally containing the seed rather than related\n" +
				"to it, `keyword broad` is the report.",
			typ: "phrase_related", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "broad", short: "Broad-match keywords containing a phrase",
			long: "Returns phrases that literally contain the seed phrase — the long-tail\n" +
				"expansion of one term, and typically the largest result set of any\n" +
				"keyword report. Only returned lines are billed, so a `--filter`\n" +
				"expression (a minimum search volume, for example) is cheaper than\n" +
				"pulling a wide page and discarding most of it.",
			typ: "phrase_fullsearch", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "questions", short: "Question keywords containing a phrase",
			long: "Returns question-form phrases built on the seed (how/what/why/where…)\n" +
				"with volume and CPC. Semrush prices this at 40 units per line, so the\n" +
				"default `--limit` of 10 is the safe pull and a wide page is a real\n" +
				"charge against the shared balance.",
			typ: "phrase_questions", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "difficulty", short: "Keyword Difficulty Index for several keywords",
			long: "Takes the phrases as separate positional arguments, joined with `;` into\n" +
				"one request, and scores each 0-100 on how hard it is to rank organically.\n" +
				"At 50 units per line this is the MOST expensive report in the tool —\n" +
				"score a shortlist already narrowed by `keyword related` or\n" +
				"`keyword broad`, never a raw dump.",
			typ: "phrase_kdi", subject: "phrase", argName: "<phrase>...", joinArg: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "organic-results", short: "Domains ranking organically for a keyword",
			long: "Returns the organic SERP for the phrase in the `--database` region, one\n" +
				"row per position with the ranking domain and URL. This describes one\n" +
				"keyword; to see everything one of those domains ranks for, follow with\n" +
				"`domain organic`.",
			typ: "phrase_organic", subject: "phrase", argName: "<phrase>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "paid-results", short: "Domains advertising on a keyword",
			long: "Returns the advertisers Semrush has seen bidding on the phrase in the\n" +
				"`--database` region, one row per ad with its copy and visible URL. A\n" +
				"phrase nobody bids on returns `row_count` 0 rather than an error.",
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
			long: "Takes a full URL including the scheme, not a bare path, and matches it\n" +
				"EXACTLY: a different scheme, host form or trailing slash is a different\n" +
				"page and comes back as `row_count` 0 rather than an error. Returns one\n" +
				"row per organic keyword attributed to that page in the `--database`\n" +
				"region. `domain pages` is how to find URLs worth asking about.",
			typ: "url_organic", subject: "url", argName: "<url>",
		}),
		s.newReportCmd(key, reportSpec{
			use: "paid", short: "Paid keywords for a specific URL",
			long: "Takes a full URL including the scheme and matches it EXACTLY, so a\n" +
				"landing page reached through a redirect or carrying tracking parameters\n" +
				"returns `row_count` 0. Returns one row per keyword whose ad lands on\n" +
				"that page in the `--database` region.",
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
			long: "Summarizes the profile in a single row: total backlinks, referring\n" +
				"domains and IPs, Authority Score, and the follow/nofollow split. One\n" +
				"line of cost, so run it before any of the paging backlink reports to\n" +
				"learn how large the profile actually is.",
			typ: "backlinks_overview", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "list", short: "Individual backlinks pointing at a target",
			long: "Returns one row per individual link — source and target URL, anchor\n" +
				"text, first and last seen dates, nofollow flag. An established domain's\n" +
				"profile runs to millions of links, so this is the most expensive way to\n" +
				"characterise one: `backlinks refdomains` answers \"who links here\" in\n" +
				"orders of magnitude fewer rows.",
			typ: "backlinks", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "refdomains", short: "Referring domains linking to a target",
			long: "Returns one row per linking domain with its Authority Score, backlink\n" +
				"count and first-seen date. Collapsing every link from a domain into one\n" +
				"row makes this the cheap read of a backlink profile and the right source\n" +
				"for a link-prospecting or outreach list.",
			typ: "backlinks_refdomains", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "anchors", short: "Anchor texts used in backlinks to a target",
			long: "Returns one row per distinct anchor text with the number of backlinks\n" +
				"and referring domains using it. The distribution is the deliverable: a\n" +
				"profile dominated by one exact-match commercial anchor reads very\n" +
				"differently from one dominated by the brand name or a bare URL.",
			typ: "backlinks_anchors", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "pages", short: "Pages of a target that receive backlinks",
			long: "Returns the target's OWN pages that attract links, one row per page with\n" +
				"its backlink and referring-domain counts. It is keyed by destination,\n" +
				"where `backlinks list` is keyed by source, so this is how to find which\n" +
				"pages a profile actually points at.",
			typ: "backlinks_pages", subject: "target", argName: "<target>", backlinks: true,
		}),
		s.newReportCmd(key, reportSpec{
			use: "competitors", short: "Domains with a similar backlink profile",
			long: "Ranks domains by how much of their referring-domain set overlaps with\n" +
				"the target's, one row per competitor with the overlap count. These are\n" +
				"link-graph neighbours and can differ sharply from the keyword-overlap\n" +
				"set `domain competitors` returns.",
			typ: "backlinks_competitors", subject: "target", argName: "<target>", backlinks: true,
		}),
	)
	return group
}
