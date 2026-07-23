package googleanalytics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// reportResponse is the subset of a runReport / runRealtimeReport response the
// human table renders.
type reportResponse struct {
	DimensionHeaders []struct {
		Name string `json:"name"`
	} `json:"dimensionHeaders"`
	MetricHeaders []struct {
		Name string `json:"name"`
	} `json:"metricHeaders"`
	Rows []struct {
		DimensionValues []struct {
			Value string `json:"value"`
		} `json:"dimensionValues"`
		MetricValues []struct {
			Value string `json:"value"`
		} `json:"metricValues"`
	} `json:"rows"`
	RowCount int `json:"rowCount"`
}

// newReportRunCmd is the core reporting verb: dimensions × metrics over date
// ranges, filters, ordering, and pagination (Data API runReport).
func (s *Service) newReportRunCmd(token string) *cobra.Command {
	var property, metrics, dimensions, startDate, endDate, filterJSON string
	var filters, orderBys []string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a GA4 report (dimensions × metrics over a date range)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			prop, err := normalizeProperty(property)
			if err != nil {
				return err
			}
			body, err := s.buildRunReportBody(cmd, metrics, dimensions, startDate, endDate, filters, filterJSON, orderBys, limit, offset)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, s.dataBase(), "/"+prop+":runReport", nil, body)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(resp)
			}
			return s.renderReportTable(resp)
		},
	}
	cmd.Flags().StringVar(&property, "property", "", "GA4 property id (numeric, or properties/<id>)")
	cmd.Flags().StringVar(&metrics, "metrics", "", "comma-separated metric API names (e.g. activeUsers,sessions)")
	cmd.Flags().StringVar(&dimensions, "dimensions", "", "comma-separated dimension API names (e.g. country,city)")
	cmd.Flags().StringVar(&startDate, "start-date", "28daysAgo", "start date (native API form: YYYY-MM-DD, NdaysAgo, yesterday, today)")
	cmd.Flags().StringVar(&endDate, "end-date", "today", "end date (native API form: YYYY-MM-DD, NdaysAgo, yesterday, today)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "dimension equality filter as dim==value (repeatable, ANDed)")
	cmd.Flags().StringVar(&filterJSON, "filter-json", "", "raw Data API FilterExpression JSON (mutually exclusive with --filter)")
	cmd.Flags().StringArrayVar(&orderBys, "order-by", nil, "ordering as metric:<name>[:asc|desc] or dimension:<name>[:asc|desc] (repeatable)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows to return (provider default 10000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "row offset for pagination")
	_ = cmd.MarkFlagRequired("property")
	_ = cmd.MarkFlagRequired("metrics")
	return cmd
}

// buildRunReportBody assembles the runReport request payload from flags.
func (s *Service) buildRunReportBody(cmd *cobra.Command, metrics, dimensions, startDate, endDate string, filters []string, filterJSON string, orderBys []string, limit, offset int) (map[string]any, error) {
	metricNames := splitNames(metrics)
	if len(metricNames) == 0 {
		return nil, &usageError{msg: "google-analytics: --metrics requires at least one metric API name (discover names with `report metadata`)"}
	}
	body := map[string]any{
		"dateRanges": []map[string]string{{"startDate": startDate, "endDate": endDate}},
		"metrics":    nameObjects("name", metricNames),
	}
	if dims := splitNames(dimensions); len(dims) > 0 {
		body["dimensions"] = nameObjects("name", dims)
	}
	filter, err := buildDimensionFilter(filters, filterJSON)
	if err != nil {
		return nil, err
	}
	if filter != nil {
		body["dimensionFilter"] = filter
	}
	if len(orderBys) > 0 {
		parsed, err := parseOrderBys(orderBys)
		if err != nil {
			return nil, err
		}
		body["orderBys"] = parsed
	}
	if cmd.Flags().Changed("limit") {
		body["limit"] = limit
	}
	if cmd.Flags().Changed("offset") {
		body["offset"] = offset
	}
	return body, nil
}

// newReportRealtimeCmd reports who is on the site right now (Data API
// runRealtimeReport; the last 30 minutes, 60 for GA 360).
func (s *Service) newReportRealtimeCmd(token string) *cobra.Command {
	var property, metrics, dimensions string
	var minutesAgo int
	cmd := &cobra.Command{
		Use:   "realtime",
		Short: "Run a GA4 realtime report (last 30 minutes; 60 for GA 360)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			prop, err := normalizeProperty(property)
			if err != nil {
				return err
			}
			metricNames := splitNames(metrics)
			if len(metricNames) == 0 {
				return &usageError{msg: "google-analytics: --metrics requires at least one metric API name"}
			}
			body := map[string]any{"metrics": nameObjects("name", metricNames)}
			if dims := splitNames(dimensions); len(dims) > 0 {
				body["dimensions"] = nameObjects("name", dims)
			}
			if cmd.Flags().Changed("minutes-ago") {
				if minutesAgo < 0 {
					return &usageError{msg: "google-analytics: --minutes-ago must be >= 0"}
				}
				body["minuteRanges"] = []map[string]int{{"startMinutesAgo": minutesAgo}}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, s.dataBase(), "/"+prop+":runRealtimeReport", nil, body)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(resp)
			}
			return s.renderReportTable(resp)
		},
	}
	cmd.Flags().StringVar(&property, "property", "", "GA4 property id (numeric, or properties/<id>)")
	cmd.Flags().StringVar(&metrics, "metrics", "", "comma-separated realtime metric API names (e.g. activeUsers)")
	cmd.Flags().StringVar(&dimensions, "dimensions", "", "comma-separated realtime dimension API names")
	cmd.Flags().IntVar(&minutesAgo, "minutes-ago", 0, "start of the minute range, minutes before now (provider default 29)")
	_ = cmd.MarkFlagRequired("property")
	_ = cmd.MarkFlagRequired("metrics")
	return cmd
}

// metadataEntry is the searchable subset of one dimension/metric metadata
// entry; raw preserves the full provider object for --json output.
type metadataEntry struct {
	APIName string `json:"apiName"`
	UIName  string `json:"uiName"`
}

// newReportMetadataCmd lists the valid dimension/metric API names (including
// custom definitions) for a property, so agents construct valid `report run`
// calls instead of guessing names.
func (s *Service) newReportMetadataCmd(token string) *cobra.Command {
	var property, kind, search string
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "List valid dimension/metric API names for a property (incl. custom definitions)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kind != "dimensions" && kind != "metrics" && kind != "all" {
				return &usageError{msg: fmt.Sprintf("google-analytics: --kind must be dimensions, metrics, or all, got %q", kind)}
			}
			prop, err := normalizeProperty(property)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.dataBase(), "/"+prop+"/metadata", nil, nil)
			if err != nil {
				return err
			}
			unfiltered := kind == "all" && search == ""
			if jsonOut(cmd) && unfiltered {
				return s.emit(body)
			}
			var resp struct {
				Name       string            `json:"name"`
				Dimensions []json.RawMessage `json:"dimensions"`
				Metrics    []json.RawMessage `json:"metrics"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return &apiError{msg: fmt.Sprintf("google-analytics: decode metadata: %v", err), err: err}
			}
			dimensions := filterMetadata(resp.Dimensions, kind == "all" || kind == "dimensions", search)
			metrics := filterMetadata(resp.Metrics, kind == "all" || kind == "metrics", search)
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{
					"name":       resp.Name,
					"dimensions": dimensions,
					"metrics":    metrics,
				})
			}
			printed := 0
			printed += s.printMetadataLines("dimension", dimensions)
			printed += s.printMetadataLines("metric", metrics)
			if printed == 0 {
				fmt.Fprintln(s.stdout(), "no matches")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&property, "property", "", "GA4 property id (numeric, or properties/<id>)")
	cmd.Flags().StringVar(&kind, "kind", "all", "which names to list: dimensions, metrics, or all")
	cmd.Flags().StringVar(&search, "search", "", "case-insensitive substring filter on apiName/uiName")
	_ = cmd.MarkFlagRequired("property")
	return cmd
}

// filterMetadata keeps the raw entries matching the kind gate + search
// substring (case-insensitive, on apiName and uiName).
func filterMetadata(entries []json.RawMessage, keep bool, search string) []json.RawMessage {
	out := []json.RawMessage{}
	if !keep {
		return out
	}
	needle := strings.ToLower(search)
	for _, raw := range entries {
		if needle != "" {
			var e metadataEntry
			if err := json.Unmarshal(raw, &e); err != nil {
				continue
			}
			if !strings.Contains(strings.ToLower(e.APIName), needle) &&
				!strings.Contains(strings.ToLower(e.UIName), needle) {
				continue
			}
		}
		out = append(out, raw)
	}
	return out
}

// printMetadataLines renders one kind's entries as kind/apiName/uiName lines
// and returns how many it printed.
func (s *Service) printMetadataLines(label string, entries []json.RawMessage) int {
	printed := 0
	for _, raw := range entries {
		var e metadataEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", label, e.APIName, e.UIName)
		printed++
	}
	return printed
}

// renderReportTable prints a report response as a compact tab-separated
// table: dimension columns then metric columns, header first.
func (s *Service) renderReportTable(body []byte) error {
	var resp reportResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return &apiError{msg: fmt.Sprintf("google-analytics: decode report: %v", err), err: err}
	}
	if len(resp.Rows) == 0 {
		fmt.Fprintln(s.stdout(), "no rows")
		return nil
	}
	header := make([]string, 0, len(resp.DimensionHeaders)+len(resp.MetricHeaders))
	for _, h := range resp.DimensionHeaders {
		header = append(header, h.Name)
	}
	for _, h := range resp.MetricHeaders {
		header = append(header, h.Name)
	}
	fmt.Fprintln(s.stdout(), strings.Join(header, "\t"))
	for _, row := range resp.Rows {
		cells := make([]string, 0, len(row.DimensionValues)+len(row.MetricValues))
		for _, v := range row.DimensionValues {
			cells = append(cells, v.Value)
		}
		for _, v := range row.MetricValues {
			cells = append(cells, v.Value)
		}
		fmt.Fprintln(s.stdout(), strings.Join(cells, "\t"))
	}
	if resp.RowCount > len(resp.Rows) {
		fmt.Fprintf(s.stdout(), "row count: %d (returned %d; paginate with --limit/--offset)\n", resp.RowCount, len(resp.Rows))
	}
	return nil
}

// splitNames parses a comma-separated name list, trimming blanks.
func splitNames(csv string) []string {
	var out []string
	for _, part := range strings.Split(csv, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// nameObjects wraps names as the Data API's [{<key>: name}, …] object list.
func nameObjects(key string, names []string) []map[string]string {
	out := make([]map[string]string, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]string{key: n})
	}
	return out
}

// buildDimensionFilter assembles the runReport dimensionFilter from the
// repeatable --filter sugar (ANDed string-equality) or the raw --filter-json
// FilterExpression. The two are mutually exclusive; both optional.
func buildDimensionFilter(filters []string, filterJSON string) (any, error) {
	if len(filters) > 0 && filterJSON != "" {
		return nil, &usageError{msg: "google-analytics: --filter and --filter-json are mutually exclusive"}
	}
	if filterJSON != "" {
		if !json.Valid([]byte(filterJSON)) {
			return nil, &usageError{msg: "google-analytics: --filter-json is not valid JSON"}
		}
		return json.RawMessage(filterJSON), nil
	}
	if len(filters) == 0 {
		return nil, nil
	}
	expressions := make([]map[string]any, 0, len(filters))
	for _, f := range filters {
		name, value, ok := strings.Cut(f, "==")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, &usageError{msg: fmt.Sprintf("google-analytics: --filter must be dimension==value, got %q", f)}
		}
		expressions = append(expressions, map[string]any{
			"filter": map[string]any{
				"fieldName":    name,
				"stringFilter": map[string]any{"matchType": "EXACT", "value": value},
			},
		})
	}
	if len(expressions) == 1 {
		return expressions[0], nil
	}
	return map[string]any{"andGroup": map[string]any{"expressions": expressions}}, nil
}

// parseOrderBys parses --order-by values of the form
// metric:<name>[:asc|desc] / dimension:<name>[:asc|desc] into Data API
// OrderBy objects. Direction defaults to asc.
func parseOrderBys(values []string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(values))
	for _, v := range values {
		parts := strings.Split(v, ":")
		if len(parts) != 2 && len(parts) != 3 {
			return nil, &usageError{msg: fmt.Sprintf("google-analytics: --order-by must be metric:<name>[:asc|desc] or dimension:<name>[:asc|desc], got %q", v)}
		}
		kind, name := parts[0], strings.TrimSpace(parts[1])
		desc := false
		if len(parts) == 3 {
			switch parts[2] {
			case "asc":
			case "desc":
				desc = true
			default:
				return nil, &usageError{msg: fmt.Sprintf("google-analytics: --order-by direction must be asc or desc, got %q", parts[2])}
			}
		}
		if name == "" {
			return nil, &usageError{msg: fmt.Sprintf("google-analytics: --order-by is missing a name in %q", v)}
		}
		var entry map[string]any
		switch kind {
		case "metric":
			entry = map[string]any{"metric": map[string]string{"metricName": name}}
		case "dimension":
			entry = map[string]any{"dimension": map[string]string{"dimensionName": name}}
		default:
			return nil, &usageError{msg: fmt.Sprintf("google-analytics: --order-by must start with metric: or dimension:, got %q", v)}
		}
		if desc {
			entry["desc"] = true
		}
		out = append(out, entry)
	}
	return out, nil
}
