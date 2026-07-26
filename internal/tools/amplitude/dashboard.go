package amplitude

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// runJSON resolves the request target, performs a GET, and passes Amplitude's
// JSON body through to stdout verbatim. Shared by every JSON-returning command.
func (s *Service) runJSON(cmd *cobra.Command, authHeader, path string, query url.Values) error {
	inv, err := s.resolve(cmd, authHeader)
	if err != nil {
		return err
	}
	body, err := s.call(cmd.Context(), inv, http.MethodGet, path, query)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// newSegmentationCmd — GET /api/2/events/segmentation. The core metric query:
// counts/uniques/props over time for one or two events. The event object(s) and
// the segment filter are large Amplitude JSON grammars, so they pass through as
// raw JSON strings (--events / --events2 / --segment) rather than being
// re-modeled here.
func (s *Service) newSegmentationCmd(authHeader string) *cobra.Command {
	var events, events2, start, end, metric, interval, segment, groupBy string
	cmd := &cobra.Command{
		Use:   "segmentation",
		Short: "Event segmentation (GET /api/2/events/segmentation)",
		Long: "The core metric query: one event's volume over time. `--events` is an\n" +
			"Amplitude event object as JSON, at minimum `{\"event_type\": \"Add to Cart\"}`,\n" +
			"and `--events2` adds a second series to the same call for comparison.\n" +
			"`--start` and `--end` are YYYYMMDD and required. `--metric` defaults to\n" +
			"`uniques` (distinct users), so `totals` is needed for raw event counts —\n" +
			"a different number, and the usual reason a result disagrees with a UI\n" +
			"chart. `--interval` is 1 (daily, the default), 7 or 30. `--segment` takes a\n" +
			"user-property filter array as JSON and `--group-by` splits the series by a\n" +
			"property. It is the cheapest of the query commands against the cost budget.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := parseJSONFlag("events", events)
			if err != nil {
				return err
			}
			if e == "" {
				return &usageError{msg: "--events is required (an Amplitude event object as JSON)"}
			}
			if start == "" || end == "" {
				return &usageError{msg: "--start and --end are required (YYYYMMDD)"}
			}
			e2, err := parseJSONFlag("events2", events2)
			if err != nil {
				return err
			}
			seg, err := parseJSONFlag("segment", segment)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("e", e)
			if e2 != "" {
				q.Set("e2", e2)
			}
			q.Set("start", start)
			q.Set("end", end)
			if metric != "" {
				q.Set("m", metric)
			}
			if interval != "" {
				q.Set("i", interval)
			}
			if seg != "" {
				q.Set("s", seg)
			}
			if groupBy != "" {
				q.Set("g", groupBy)
			}
			return s.runJSON(cmd, authHeader, "/api/2/events/segmentation", q)
		},
	}
	f := cmd.Flags()
	f.StringVar(&events, "events", "", "primary event object as JSON (required)")
	f.StringVar(&events2, "events2", "", "second event object as JSON (optional)")
	f.StringVar(&start, "start", "", "first date YYYYMMDD (required)")
	f.StringVar(&end, "end", "", "last date YYYYMMDD (required)")
	f.StringVarP(&metric, "metric", "m", "", "metric: uniques|totals|pct_dau|average|... (default uniques)")
	f.StringVarP(&interval, "interval", "i", "", "interval: 1|7|30 or -300000|-3600000 (default 1)")
	f.StringVarP(&segment, "segment", "s", "", "segment definition as JSON (optional)")
	f.StringVarP(&groupBy, "group-by", "g", "", "group-by property (optional)")
	return cmd
}

// newFunnelsCmd — GET /api/2/funnels. Conversion / drop-off across an ordered
// event list. Amplitude takes one `e` query param per funnel step, so --events
// is repeatable (one JSON event object per occurrence, in order).
func (s *Service) newFunnelsCmd(authHeader string) *cobra.Command {
	var events []string
	var start, end, mode, interval, segment string
	cmd := &cobra.Command{
		Use:   "funnels",
		Short: "Funnel conversion (GET /api/2/funnels)",
		Long: "One `--events` per step, repeated, and the FLAG ORDER IS THE FUNNEL — at\n" +
			"least two are required and swapping them measures a different journey.\n" +
			"Each value is an Amplitude event object as JSON. `--start` and `--end` are\n" +
			"YYYYMMDD. `--mode` chooses the conversion semantics — `ordered`,\n" +
			"`unordered` or `sequential` — which changes what counts as a conversion and\n" +
			"therefore the numbers, so report which mode produced them. Funnels are\n" +
			"among the most expensive queries against the cost budget: narrow the date\n" +
			"range before widening the step list.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(events) < 2 {
				return &usageError{msg: "--events must be given at least twice (one JSON event object per funnel step, in order)"}
			}
			if start == "" || end == "" {
				return &usageError{msg: "--start and --end are required (YYYYMMDD)"}
			}
			q := url.Values{}
			for _, raw := range events {
				step, err := parseJSONFlag("events", raw)
				if err != nil {
					return err
				}
				if step == "" {
					return &usageError{msg: "each --events value must be a non-empty JSON event object"}
				}
				q.Add("e", step)
			}
			seg, err := parseJSONFlag("segment", segment)
			if err != nil {
				return err
			}
			q.Set("start", start)
			q.Set("end", end)
			if mode != "" {
				q.Set("mode", mode)
			}
			if interval != "" {
				q.Set("i", interval)
			}
			if seg != "" {
				q.Set("s", seg)
			}
			return s.runJSON(cmd, authHeader, "/api/2/funnels", q)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&events, "events", nil, "funnel-step event object as JSON (repeatable, ordered, >=2)")
	f.StringVar(&start, "start", "", "first date YYYYMMDD (required)")
	f.StringVar(&end, "end", "", "last date YYYYMMDD (required)")
	f.StringVar(&mode, "mode", "", "conversion window mode: ordered|unordered|sequential (optional)")
	f.StringVarP(&interval, "interval", "i", "", "interval: 1|7|30 (optional)")
	f.StringVarP(&segment, "segment", "s", "", "segment definition as JSON (optional)")
	return cmd
}

// newRetentionCmd — GET /api/2/retention. N-day / bracket retention. Retention
// uses a start event (se) and a returning event (re), not the segmentation
// `e`/`s` pair.
func (s *Service) newRetentionCmd(authHeader string) *cobra.Command {
	var startEvent, returningEvent, start, end, interval, retentionType, segment string
	cmd := &cobra.Command{
		Use:   "retention",
		Short: "Retention analysis (GET /api/2/retention)",
		Long: "Takes a DIFFERENT flag pair from the other queries: `--start-event` and\n" +
			"`--returning-event`, each an Amplitude event object as JSON — `--events` is\n" +
			"not accepted here. `--retention-type` picks `n-day` (returned exactly on\n" +
			"day N), `unbounded` (on or after day N) or `bracket`, and the same data\n" +
			"yields materially different curves under each, so a retention number is\n" +
			"meaningless without saying which. `--start` and `--end` (YYYYMMDD) bound\n" +
			"the cohort's starting window, not the return window. Expensive against the\n" +
			"cost budget.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			se, err := parseJSONFlag("start-event", startEvent)
			if err != nil {
				return err
			}
			re, err := parseJSONFlag("returning-event", returningEvent)
			if err != nil {
				return err
			}
			if se == "" || re == "" {
				return &usageError{msg: "--start-event and --returning-event are required (Amplitude event objects as JSON)"}
			}
			if start == "" || end == "" {
				return &usageError{msg: "--start and --end are required (YYYYMMDD)"}
			}
			seg, err := parseJSONFlag("segment", segment)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("se", se)
			q.Set("re", re)
			q.Set("start", start)
			q.Set("end", end)
			if interval != "" {
				q.Set("i", interval)
			}
			if retentionType != "" {
				q.Set("rm", retentionType)
			}
			if seg != "" {
				q.Set("s", seg)
			}
			return s.runJSON(cmd, authHeader, "/api/2/retention", q)
		},
	}
	f := cmd.Flags()
	f.StringVar(&startEvent, "start-event", "", "start event object as JSON (required)")
	f.StringVar(&returningEvent, "returning-event", "", "returning event object as JSON (required)")
	f.StringVar(&start, "start", "", "first date YYYYMMDD (required)")
	f.StringVar(&end, "end", "", "last date YYYYMMDD (required)")
	f.StringVarP(&interval, "interval", "i", "", "interval: 1|7|30 (optional)")
	f.StringVar(&retentionType, "retention-type", "", "retention mode rm: n-day|unbounded|bracket (optional)")
	f.StringVarP(&segment, "segment", "s", "", "segment definition as JSON (optional)")
	return cmd
}

// newEventsListCmd — GET /api/2/events/list. Catalog of tracked event types
// (the prerequisite for building valid segmentation/funnels queries). No params.
func (s *Service) newEventsListCmd(authHeader string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all tracked event types (GET /api/2/events/list)",
		Long: "The catalog of event types this project actually tracks, and the right\n" +
			"first call of any analysis: `segmentation`, `funnels` and `retention` all\n" +
			"take an `event_type` string that must match one of these EXACTLY, and a\n" +
			"near-miss returns an empty result rather than an error. Takes no\n" +
			"parameters and is cheap.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runJSON(cmd, authHeader, "/api/2/events/list", nil)
		},
	}
}

// newUserSearchCmd — GET /api/2/usersearch. Resolve a user / device / email /
// prefix to an Amplitude ID (the id useractivity requires).
func (s *Service) newUserSearchCmd(authHeader string) *cobra.Command {
	var user string
	cmd := &cobra.Command{
		Use:   "user-search",
		Short: "Search for a user by Amplitude ID, Device ID, User ID, or prefix (GET /api/2/usersearch)",
		Long: "Resolves an email, a product-side user id, a device id or a prefix into the\n" +
			"AMPLITUDE ID that `user-activity` requires — that command takes nothing\n" +
			"else. A prefix can match several people, so the result is a list to choose\n" +
			"from rather than a single answer, and an unknown identifier comes back\n" +
			"empty rather than failing.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if user == "" {
				return &usageError{msg: "--user is required"}
			}
			q := url.Values{}
			q.Set("user", user)
			return s.runJSON(cmd, authHeader, "/api/2/usersearch", q)
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "Amplitude ID, Device ID, User ID, or prefix (required)")
	return cmd
}

// newUserActivityCmd — GET /api/2/useractivity. A user's raw event stream by
// Amplitude ID. --offset / --limit page the stream.
func (s *Service) newUserActivityCmd(authHeader string) *cobra.Command {
	var user, offset, limit string
	cmd := &cobra.Command{
		Use:   "user-activity",
		Short: "A user's event stream by Amplitude ID (GET /api/2/useractivity)",
		Long: "`--user` must be the Amplitude ID from `user-search`; an email or the\n" +
			"product's own user id will not resolve here. Returns ONE person's raw event\n" +
			"stream and user properties — individual behaviour, not an aggregate, and\n" +
			"the most privacy-sensitive read in this tool alongside `export`. The stream\n" +
			"is bounded per call: `--limit` and `--offset` page further back.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if user == "" {
				return &usageError{msg: "--user is required (an Amplitude ID from user-search)"}
			}
			q := url.Values{}
			q.Set("user", user)
			if offset != "" {
				q.Set("offset", offset)
			}
			if limit != "" {
				q.Set("limit", limit)
			}
			return s.runJSON(cmd, authHeader, "/api/2/useractivity", q)
		},
	}
	f := cmd.Flags()
	f.StringVar(&user, "user", "", "Amplitude ID (required)")
	f.StringVar(&offset, "offset", "", "event-stream offset (optional)")
	f.StringVar(&limit, "limit", "", "max events to return (optional)")
	return cmd
}

// newCohortsListCmd — GET /api/3/cohorts. Discoverable behavioral cohorts
// (metadata only; member download is out of v1 scope). No params.
func (s *Service) newCohortsListCmd(authHeader string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all discoverable behavioral cohorts (GET /api/3/cohorts)",
		Long: "Metadata only — names, ids, sizes and definitions of the cohorts marked\n" +
			"discoverable in the project. Member lists cannot be downloaded through this\n" +
			"tool. Read it before hand-building a `--segment` filter: the team may\n" +
			"already have defined the population in question, and its definition here\n" +
			"says how they define it.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runJSON(cmd, authHeader, "/api/3/cohorts", nil)
		},
	}
}
