package sproutsocial

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// queryFlags carries the thin ergonomic flags the POST-with-a-filter-body
// endpoints (analytics, messages, cases) share. build() assembles the Sprout
// request body from whichever flags a command registered; the --body escape
// hatch, when set, replaces the whole assembled body with a verbatim JSON
// object so any documented Sprout query can be sent unchanged.
type queryFlags struct {
	filters    []string
	metrics    []string
	fields     string // comma-separated → fields array
	sort       []string
	page       int
	limit      int
	pageCursor string
	timezone   string
	body       string // raw JSON object; overrides all of the above
}

// build returns the request payload. With --body set it parses and returns that
// object verbatim (a non-object or invalid JSON is a usage error). Otherwise it
// assembles a map from the set flags only — Sprout requires `filters` on the
// inbox/cases endpoints, and the API's own 400 surfaces (passed through) when a
// required field is missing.
func (q queryFlags) build() (any, error) {
	if q.body != "" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(q.body), &raw); err != nil {
			return nil, &usageError{msg: fmt.Sprintf("--body is not a valid JSON object: %v", err)}
		}
		return raw, nil
	}
	m := map[string]any{}
	if len(q.filters) > 0 {
		m["filters"] = q.filters
	}
	if len(q.metrics) > 0 {
		m["metrics"] = q.metrics
	}
	if f := splitComma(q.fields); len(f) > 0 {
		m["fields"] = f
	}
	if len(q.sort) > 0 {
		m["sort"] = q.sort
	}
	if q.page > 0 {
		m["page"] = q.page
	}
	if q.limit > 0 {
		m["limit"] = q.limit
	}
	if q.pageCursor != "" {
		m["page_cursor"] = q.pageCursor
	}
	if q.timezone != "" {
		m["timezone"] = q.timezone
	}
	return m, nil
}

// registerFilterFlags wires the flags common to every POST-filter verb.
func registerFilterFlags(cmd *cobra.Command, q *queryFlags) {
	cmd.Flags().StringArrayVar(&q.filters, "filter", nil, "Sprout filter DSL clause, e.g. created_time.in(2026-01-01...2026-02-01) (repeatable)")
	cmd.Flags().StringVar(&q.fields, "fields", "", "comma-separated response fields to include")
	cmd.Flags().StringArrayVar(&q.sort, "sort", nil, "sort clause, e.g. created_time:desc (repeatable)")
	cmd.Flags().StringVar(&q.timezone, "timezone", "", "IANA timezone for date filters/grouping")
	cmd.Flags().StringVar(&q.body, "body", "", "raw JSON request body (verbatim passthrough; overrides the flags above)")
}

// newAnalyticsCmd is the analytics group: profile- and post-level metrics, both
// POST /v1/{cid}/analytics/<r> with index-based (page/limit) paging.
func (s *Service) newAnalyticsCmd(token string) *cobra.Command {
	cmd := newGroupCmd("analytics", "Read profile and post analytics (POST filter endpoints)")
	cmd.AddCommand(
		s.newAnalyticsPostCmd(token, "profiles", "analytics/profiles", "Profile-level analytics (POST /v1/{cid}/analytics/profiles)", longAnalyticsProfiles),
		s.newAnalyticsPostCmd(token, "posts", "analytics/posts", "Post-level analytics (POST /v1/{cid}/analytics/posts)", longAnalyticsPosts),
	)
	return cmd
}

// longAnalyticsProfiles and longAnalyticsPosts are the two analytics Longs.
// They live beside the shared builder because it is the builder that fixes the
// filter-body contract and the index paging both of them describe; only the
// grain of a row differs.
const (
	longAnalyticsProfiles = "Aggregates by social PROFILE — one row per connected account rather than per\n" +
		"post — so the result set is bounded by how many profiles the customer has,\n" +
		"not by the date window. `--metric` is repeatable and names the Sprout\n" +
		"metrics to return; omitting it takes the endpoint's defaults. Index-paged:\n" +
		"`--page` is 1-based, `--limit` sizes it, and `paging.total_pages` says when\n" +
		"to stop. Profile ids come from `metadata profiles`."

	longAnalyticsPosts = "One row per published post, so the result set grows with the date window —\n" +
		"bound `created_time` in a `--filter` before widening anything else.\n" +
		"`--metric` is repeatable and `--fields` trims each row (post text,\n" +
		"permalink) to keep pages small. Index-paged through `--page` / `--limit`\n" +
		"against `paging.total_pages`, not the cursor `messages list` uses. Covers\n" +
		"posts Sprout published or is tracking; it is not a general search of a\n" +
		"network."
)

// newAnalyticsPostCmd builds one analytics verb. Analytics uses index-based
// paging (--page / --limit) and accepts --metric (repeatable).
func (s *Service) newAnalyticsPostCmd(token, use, resource, short, long string) *cobra.Command {
	var q queryFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cid, err := resolveCID(cmd)
			if err != nil {
				return err
			}
			payload, err := q.build()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/"+cid+"/"+resource, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerFilterFlags(cmd, &q)
	cmd.Flags().StringArrayVar(&q.metrics, "metric", nil, "metric to request, e.g. impressions (repeatable)")
	cmd.Flags().IntVar(&q.page, "page", 0, "1-based page index (index paging)")
	cmd.Flags().IntVar(&q.limit, "limit", 0, "max results per page")
	return cmd
}

// newMessagesCmd is the inbox group. `messages list` is POST /v1/{cid}/messages
// with cursor-based paging (--page-cursor / paging.next_cursor).
func (s *Service) newMessagesCmd(token string) *cobra.Command {
	cmd := newGroupCmd("messages", "Triage the shared inbox (POST /v1/{cid}/messages)")
	cmd.AddCommand(s.newMessagesListCmd(token))
	return cmd
}

func (s *Service) newMessagesListCmd(token string) *cobra.Command {
	var q queryFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List inbox messages (POST /v1/{cid}/messages)",
		Long: "At least one `--filter` is required, and a time-bounded one is the usual\n" +
			"entry point (`created_time.in(2026-01-01...2026-01-02)`). `--limit` defaults\n" +
			"to 50 and Sprout caps it at 100. Paging is CURSOR-based — feed\n" +
			"`paging.next_cursor` back through `--page-cursor`; the `--page` flag that\n" +
			"analytics uses does not exist here. This reads the shared inbox only:\n" +
			"nothing in this tool replies to, assigns or closes a message.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cid, err := resolveCID(cmd)
			if err != nil {
				return err
			}
			payload, err := q.build()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/"+cid+"/messages", payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerFilterFlags(cmd, &q)
	cmd.Flags().IntVar(&q.limit, "limit", 0, "max results per page (default 50, max 100)")
	cmd.Flags().StringVar(&q.pageCursor, "page-cursor", "", "resume from a prior response's paging.next_cursor")
	return cmd
}

// newCasesCmd is the cases group. `cases filter` is POST /v1/{cid}/cases/filter
// with cursor-based paging.
func (s *Service) newCasesCmd(token string) *cobra.Command {
	cmd := newGroupCmd("cases", "Query support cases (POST /v1/{cid}/cases/filter)")
	cmd.AddCommand(s.newCasesFilterCmd(token))
	return cmd
}

func (s *Service) newCasesFilterCmd(token string) *cobra.Command {
	var q queryFlags
	cmd := &cobra.Command{
		Use:   "filter",
		Short: "Filter support cases (POST /v1/{cid}/cases/filter)",
		Long: "Cases are distinct objects from inbox messages — a case id is not a message\n" +
			"id and the two filter vocabularies do not overlap. At least one `--filter`\n" +
			"is required. `--limit` defaults to 50 with a Sprout cap of 100, and paging\n" +
			"is `--page-cursor` fed from `paging.next_cursor`. Read-only: no case can be\n" +
			"opened, assigned or resolved from here.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cid, err := resolveCID(cmd)
			if err != nil {
				return err
			}
			payload, err := q.build()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/"+cid+"/cases/filter", payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerFilterFlags(cmd, &q)
	cmd.Flags().IntVar(&q.limit, "limit", 0, "max results per page (default 50, max 100)")
	cmd.Flags().StringVar(&q.pageCursor, "page-cursor", "", "resume from a prior response's paging.next_cursor")
	return cmd
}
