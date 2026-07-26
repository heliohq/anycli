package moz

import (
	"github.com/spf13/cobra"
)

// validScopes are the site/target query scopes the Moz API accepts. An empty
// scope is also allowed (the API applies its own default); a non-empty value
// is validated here so a typo becomes a usage error (exit 2) rather than a
// quota-billing round trip.
var validScopes = map[string]struct{}{
	"page":        {},
	"subdomain":   {},
	"root_domain": {},
}

// siteQuery builds a Moz site_query/target_query object from a URL and an
// optional scope. When scope is empty it is omitted so the API applies its
// documented default.
func siteQuery(query, scope string) map[string]any {
	q := map[string]any{"query": query}
	if scope != "" {
		q["scope"] = scope
	}
	return q
}

// checkScope validates an optional scope flag value.
func checkScope(scope string) error {
	if scope == "" {
		return nil
	}
	if _, ok := validScopes[scope]; !ok {
		return &usageError{msg: "moz: --scope must be one of page, subdomain, root_domain"}
	}
	return nil
}

func (s *Service) newSiteCmd(token string) *cobra.Command {
	cmd := newGroupCmd("site", "Site authority, brand authority, and top pages")
	cmd.AddCommand(
		s.newSiteMetricsCmd(token),
		s.newSiteBrandAuthorityCmd(token),
		s.newSiteTopPagesCmd(token),
	)
	return cmd
}

// newSiteMetricsCmd fetches DA/PA/spam and link counts for one URL
// (data.site.metrics.fetch), or for several URLs in a single billed call when
// --site is repeated (data.site.metrics.fetch.multiple).
func (s *Service) newSiteMetricsCmd(token string) *cobra.Command {
	var sites []string
	var scope string
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Domain/Page Authority, spam score, and link counts for a URL (repeat --site for a batch)",
		Long: "Repeating --site switches to Moz's batch method, so several URLs travel\n" +
			"in ONE request instead of a loop; the quota still scales with the URLs,\n" +
			"the round trips do not. --scope decides what each string means, and the\n" +
			"same URL answers very differently under page and root_domain — a page's\n" +
			"Page Authority is not its site's Domain Authority.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(sites) == 0 {
				return &usageError{msg: "moz: at least one --site is required"}
			}
			if err := checkScope(scope); err != nil {
				return err
			}
			if len(sites) == 1 {
				data := map[string]any{"site_query": siteQuery(sites[0], scope)}
				result, err := s.call(cmd.Context(), token, "data.site.metrics.fetch", data)
				if err != nil {
					return err
				}
				return s.emit(result)
			}
			queries := make([]map[string]any, 0, len(sites))
			for _, site := range sites {
				queries = append(queries, siteQuery(site, scope))
			}
			data := map[string]any{"site_queries": queries}
			result, err := s.call(cmd.Context(), token, "data.site.metrics.fetch.multiple", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringArrayVar(&sites, "site", nil, "URL/domain to fetch metrics for (repeatable for a single batched call)")
	cmd.Flags().StringVar(&scope, "scope", "", "query scope: page|subdomain|root_domain (default: API default)")
	return cmd
}

// newSiteBrandAuthorityCmd fetches the Brand Authority score for a domain
// (data.site.metrics.brand.authority.fetch). Brand Authority is domain-level,
// so it takes no scope.
func (s *Service) newSiteBrandAuthorityCmd(token string) *cobra.Command {
	var site string
	cmd := &cobra.Command{
		Use:   "brand-authority",
		Short: "Brand Authority score for a domain",
		Long: "Brand Authority is domain-level only, which is why this command has no\n" +
			"--scope: a URL with a path is still scored as its domain. It reflects\n" +
			"how much branded demand a name attracts, a different signal from the\n" +
			"link-based Domain Authority in `site metrics` — a site can score well on\n" +
			"one and poorly on the other.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if site == "" {
				return &usageError{msg: "moz: --site is required"}
			}
			data := map[string]any{"site_query": map[string]any{"query": site}}
			result, err := s.call(cmd.Context(), token, "data.site.metrics.brand.authority.fetch", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "domain to fetch Brand Authority for")
	return cmd
}

// newSiteTopPagesCmd lists a site's top pages by authority
// (data.site.top-page.list).
func (s *Service) newSiteTopPagesCmd(token string) *cobra.Command {
	var site, scope string
	var limit int
	cmd := &cobra.Command{
		Use:   "top-pages",
		Short: "Top pages for a site, ranked by authority",
		Long: "Ranked by each page's own authority, which shows where link equity has\n" +
			"accumulated — not where traffic goes; Moz has no traffic data. Pass\n" +
			"--scope root_domain or subdomain, since page scope narrows the target to\n" +
			"a single URL and makes the list pointless. --limit defaults to 25 and\n" +
			"every returned page bills a row.",
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
			result, err := s.call(cmd.Context(), token, "data.site.top-page.list", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "site to list top pages for")
	cmd.Flags().StringVar(&scope, "scope", "", "query scope: page|subdomain|root_domain (default: API default)")
	cmd.Flags().IntVar(&limit, "limit", 25, "max pages to return (each returned row bills quota)")
	return cmd
}
