package mixpanel

import (
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// projectValues seeds a query.Values with the injected project_id, which every
// Query API call requires (a Service Account can see several projects, so the
// project is not implied by the credential).
func (c *client) projectValues() url.Values {
	v := url.Values{}
	v.Set("project_id", c.projectID)
	return v
}

// setIf adds key=val only when val is non-empty, keeping optional analytical
// params out of the query string when unset.
func setIf(v url.Values, key, val string) {
	if val != "" {
		v.Set(key, val)
	}
}

// jsonArray marshals a repeated string flag to the JSON-array string Mixpanel
// expects for multi-event parameters (e.g. `event=["A","B"]`).
func jsonArray(items []string) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// newSegmentationCmd — GET /api/query/segmentation.
func (s *Service) newSegmentationCmd(c *client) *cobra.Command {
	var event, from, to, on, where, typ, unit string
	cmd := &cobra.Command{
		Use:   "segmentation",
		Short: "Segment an event over time (GET /segmentation)",
		Long: "`--event`, `--from` and `--to` are all required here, and `--event` takes a\n" +
			"single name — unlike `events`, which is repeatable and can leave the window\n" +
			"implicit. `--on` and `--where` are Mixpanel expressions over event\n" +
			"properties (`properties[\"$browser\"]`), not bare property names. `--type`\n" +
			"picks general (event count), unique (distinct users) or average, and\n" +
			"`--unit` buckets the series from minute up to month.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := c.projectValues()
			v.Set("event", event)
			v.Set("from_date", from)
			v.Set("to_date", to)
			setIf(v, "on", on)
			setIf(v, "where", where)
			setIf(v, "type", typ)
			setIf(v, "unit", unit)
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/segmentation", v)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&event, "event", "", "event name to segment (required)")
	f.StringVar(&from, "from", "", "start date YYYY-MM-DD (required)")
	f.StringVar(&to, "to", "", "end date YYYY-MM-DD (required)")
	f.StringVar(&on, "on", "", "property expression to segment on")
	f.StringVar(&where, "where", "", "filter expression")
	f.StringVar(&typ, "type", "", "general | unique | average")
	f.StringVar(&unit, "unit", "", "minute | hour | day | week | month")
	_ = cmd.MarkFlagRequired("event")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// newEventsCmd — GET /api/query/events (event totals over time).
func (s *Service) newEventsCmd(c *client) *cobra.Command {
	var events []string
	var typ, unit, interval, from, to string
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Event totals over time (GET /events)",
		Long: "`--event` is repeatable and is sent as one JSON array, so several series\n" +
			"come back for a single query against the hourly budget — cheaper than one\n" +
			"call per event. Bound the window EITHER with `--interval` (that many\n" +
			"`--unit`s back from today) OR with `--from`/`--to`; pick one rather than\n" +
			"mixing them. `--type` is general (event count), unique (distinct users) or\n" +
			"average.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			arr, err := jsonArray(events)
			if err != nil {
				return &usageError{msg: "encode --event list: " + err.Error()}
			}
			v := c.projectValues()
			v.Set("event", arr)
			setIf(v, "type", typ)
			setIf(v, "unit", unit)
			setIf(v, "interval", interval)
			setIf(v, "from_date", from)
			setIf(v, "to_date", to)
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/events", v)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&events, "event", nil, "event name (repeatable, required)")
	f.StringVar(&typ, "type", "", "general | unique | average")
	f.StringVar(&unit, "unit", "", "minute | hour | day | week | month")
	f.StringVar(&interval, "interval", "", "number of units back from today")
	f.StringVar(&from, "from", "", "start date YYYY-MM-DD (use with --to instead of --interval)")
	f.StringVar(&to, "to", "", "end date YYYY-MM-DD")
	_ = cmd.MarkFlagRequired("event")
	return cmd
}

// newEventsNamesCmd — GET /api/query/events/names. Primary event-name
// discovery: the actively-firing event names (use this, not lexicon).
func (s *Service) newEventsNamesCmd(c *client) *cobra.Command {
	var typ, limit string
	cmd := &cobra.Command{
		Use:   "events-names",
		Short: "List actively-firing event names — primary discovery (GET /events/names)",
		Long: "The authoritative inventory of what the project actually records, which\n" +
			"`lexicon list` cannot supply. Run it before any verb that takes `--event`:\n" +
			"those verbs match names exactly, with no fuzzy resolution. `--limit` caps\n" +
			"how many names return and `--type` is the same general | unique | average\n" +
			"switch as `events`, deciding what the counts behind the ordering measure.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := c.projectValues()
			setIf(v, "type", typ)
			setIf(v, "limit", limit)
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/events/names", v)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&typ, "type", "", "general | unique | average")
	f.StringVar(&limit, "limit", "", "max number of event names to return")
	return cmd
}

// newFunnelsListCmd — GET /api/query/funnels/list (saved funnels: id + name).
func (s *Service) newFunnelsListCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved funnels (GET /funnels/list)",
		Long: "Returns id and name only, and only for funnels SAVED in the Mixpanel UI —\n" +
			"an ad-hoc funnel cannot be defined from here. `funnels run` needs an id from\n" +
			"this list and there is no other source for one.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/funnels/list", c.projectValues())
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
}

// newFunnelsRunCmd — GET /api/query/funnels (run a saved funnel).
func (s *Service) newFunnelsRunCmd(c *client) *cobra.Command {
	var funnelID, from, to, on, where string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a saved funnel by id (GET /funnels)",
		Long: "The step definition lives in Mixpanel and cannot be overridden here — only\n" +
			"the window and the slicing are yours. `--funnel-id` comes from\n" +
			"`funnels list`. `--from`/`--to` are optional and fall back to the saved\n" +
			"report's own window. `--on` breaks the conversion down by a property\n" +
			"expression and `--where` narrows the population, both in Mixpanel's\n" +
			"expression syntax.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := c.projectValues()
			v.Set("funnel_id", funnelID)
			setIf(v, "from_date", from)
			setIf(v, "to_date", to)
			setIf(v, "on", on)
			setIf(v, "where", where)
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/funnels", v)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&funnelID, "funnel-id", "", "saved funnel id (required)")
	f.StringVar(&from, "from", "", "start date YYYY-MM-DD")
	f.StringVar(&to, "to", "", "end date YYYY-MM-DD")
	f.StringVar(&on, "on", "", "property expression to segment on")
	f.StringVar(&where, "where", "", "filter expression")
	_ = cmd.MarkFlagRequired("funnel-id")
	return cmd
}

// newRetentionCmd — GET /api/query/retention.
func (s *Service) newRetentionCmd(c *client) *cobra.Command {
	var from, to, bornEvent, event, retentionType, interval, unit string
	cmd := &cobra.Command{
		Use:   "retention",
		Short: "Cohort retention over time (GET /retention)",
		Long: "`--born-event` is what puts a user INTO the cohort; `--event` is what counts\n" +
			"as coming back. Leaving either out widens it to any event, which is almost\n" +
			"never the question being asked. `--retention-type` is birth (a first-time\n" +
			"cohort) or compounded. `--interval` and `--unit` set the bucket width\n" +
			"together. No flag is strictly required, but without `--from`/`--to` the\n" +
			"window is Mixpanel's default rather than a chosen one.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := c.projectValues()
			setIf(v, "from_date", from)
			setIf(v, "to_date", to)
			setIf(v, "born_event", bornEvent)
			setIf(v, "event", event)
			setIf(v, "retention_type", retentionType)
			setIf(v, "interval", interval)
			setIf(v, "unit", unit)
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/retention", v)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&from, "from", "", "start date YYYY-MM-DD")
	f.StringVar(&to, "to", "", "end date YYYY-MM-DD")
	f.StringVar(&bornEvent, "born-event", "", "the first ('born') event")
	f.StringVar(&event, "event", "", "the returning event")
	f.StringVar(&retentionType, "retention-type", "", "birth | compounded")
	f.StringVar(&interval, "interval", "", "number of units per retention bucket")
	f.StringVar(&unit, "unit", "", "day | week | month")
	return cmd
}

// newRetentionFrequencyCmd — GET /api/query/retention/addiction (frequency view).
func (s *Service) newRetentionFrequencyCmd(c *client) *cobra.Command {
	var from, to, event, unit, bornEvent string
	cmd := &cobra.Command{
		Use:   "retention-frequency",
		Short: "Retention frequency / 'addiction' view (GET /retention/addiction)",
		Long: "Answers how MANY units a cohort was active in, rather than whether it\n" +
			"returned at a given offset — a different question from `retention`, not a\n" +
			"different formatting of it. Takes the same `--born-event` / `--event` /\n" +
			"`--unit` vocabulary, but has no `--retention-type` or `--interval`: the\n" +
			"buckets are counts of active units and the endpoint fixes them.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := c.projectValues()
			setIf(v, "from_date", from)
			setIf(v, "to_date", to)
			setIf(v, "event", event)
			setIf(v, "unit", unit)
			setIf(v, "born_event", bornEvent)
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/retention/addiction", v)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&from, "from", "", "start date YYYY-MM-DD")
	f.StringVar(&to, "to", "", "end date YYYY-MM-DD")
	f.StringVar(&event, "event", "", "the returning event")
	f.StringVar(&unit, "unit", "", "day | week | month")
	f.StringVar(&bornEvent, "born-event", "", "the first ('born') event")
	return cmd
}

// newInsightsCmd — GET /api/query/insights (fetch a saved Insights report).
func (s *Service) newInsightsCmd(c *client) *cobra.Command {
	var bookmarkID string
	cmd := &cobra.Command{
		Use:   "insights",
		Short: "Fetch a saved Insights report by bookmark id (GET /insights)",
		Long: "`--bookmark-id` identifies a report saved in Mixpanel's Insights UI — it is\n" +
			"not an event name, and nothing here lists bookmarks or creates one, so the\n" +
			"id has to be read out of the report's own URL. The response follows that\n" +
			"report's stored configuration: changing the metric, breakdown or window\n" +
			"means editing the report in Mixpanel, not passing different flags.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := c.projectValues()
			v.Set("bookmark_id", bookmarkID)
			body, err := c.getJSON(cmd.Context(), c.queryBase, "/insights", v)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&bookmarkID, "bookmark-id", "", "saved Insights report bookmark id (required)")
	_ = cmd.MarkFlagRequired("bookmark-id")
	return cmd
}

// newCohortsListCmd — POST /api/query/cohorts/list. Mixpanel defines this POST
// even though the current surface exposes no body params; project_id stays in
// the query string.
func (s *Service) newCohortsListCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved cohorts (POST /cohorts/list)",
		Long: "A POST despite being a pure read, and it accepts no filters or paging — it\n" +
			"returns every saved cohort in the project with its id and name. Cohorts are\n" +
			"authored in the Mixpanel UI; this verb only names them and cannot create,\n" +
			"edit or evaluate one.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.postForm(cmd.Context(), c.queryBase, "/cohorts/list", c.projectValues(), nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
}

// newEngageCmd — POST /api/query/engage (query People / user profiles).
// project_id stays in the query string; the analytical params travel in a
// form-encoded request body.
func (s *Service) newEngageCmd(c *client) *cobra.Command {
	var where string
	var outputProps []string
	var page string
	cmd := &cobra.Command{
		Use:   "engage",
		Short: "Query People / user profiles (POST /engage)",
		Long: "Queries People PROFILES, not events, so `--where` is an expression over\n" +
			"profile properties and an event-property expression matches nothing here.\n" +
			"`--output-properties` is repeatable and trims each profile to those keys,\n" +
			"which is worth doing because profiles are wide. `--page` is ZERO-indexed and\n" +
			"must be an integer (checked locally); the page size is Mixpanel's and this\n" +
			"command cannot change it.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			setIf(form, "where", where)
			if len(outputProps) > 0 {
				arr, err := jsonArray(outputProps)
				if err != nil {
					return &usageError{msg: "encode --output-properties list: " + err.Error()}
				}
				form.Set("output_properties", arr)
			}
			if page != "" {
				if _, err := strconv.Atoi(page); err != nil {
					return &usageError{msg: "--page must be an integer"}
				}
				form.Set("page", page)
			}
			body, err := c.postForm(cmd.Context(), c.queryBase, "/engage", c.projectValues(), form)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&where, "where", "", "filter expression over profile properties")
	f.StringArrayVar(&outputProps, "output-properties", nil, "profile property to include (repeatable)")
	f.StringVar(&page, "page", "", "zero-indexed page for paging through results")
	return cmd
}
