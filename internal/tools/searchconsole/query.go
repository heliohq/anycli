package searchconsole

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ptLocation is the fixed time zone Search Console reports dates in. --days
// windows are computed against it so they match the API's own day boundaries.
const ptZone = "America/Los_Angeles"

// dimensionFilter is one filter clause parsed from a --filter value.
type dimensionFilter struct {
	Dimension  string `json:"dimension"`
	Operator   string `json:"operator"`
	Expression string `json:"expression"`
}

// dimensionFilterGroup wraps the filters; the API supports only groupType "and".
type dimensionFilterGroup struct {
	GroupType string            `json:"groupType"`
	Filters   []dimensionFilter `json:"filters"`
}

// queryRequest is the searchAnalytics.query body. Optional knobs use omitempty
// so the API applies its own defaults (rowLimit 1000, dataState final, type web)
// rather than receiving a zero value.
type queryRequest struct {
	StartDate             string                 `json:"startDate"`
	EndDate               string                 `json:"endDate"`
	Dimensions            []string               `json:"dimensions,omitempty"`
	Type                  string                 `json:"type,omitempty"`
	DimensionFilterGroups []dimensionFilterGroup `json:"dimensionFilterGroups,omitempty"`
	RowLimit              int                    `json:"rowLimit,omitempty"`
	StartRow              int                    `json:"startRow,omitempty"`
	DataState             string                 `json:"dataState,omitempty"`
	AggregationType       string                 `json:"aggregationType,omitempty"`
}

func (s *Service) newQueryCmd(token string) *cobra.Command {
	var (
		site        string
		start, end  string
		days        int
		dimensions  string
		typ         string
		filters     []string
		rowLimit    int
		startRow    int
		dataState   string
		aggregation string
	)
	cmd := &cobra.Command{
		Use:         "query",
		Short:       "Search analytics: clicks/impressions/CTR/position by dimension",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if site == "" {
				return &usageError{msg: "--site is required"}
			}
			resolvedStart, resolvedEnd, err := s.resolveWindow(start, end, days)
			if err != nil {
				return err
			}
			groups, err := parseFilters(filters)
			if err != nil {
				return err
			}
			req := queryRequest{
				StartDate:             resolvedStart,
				EndDate:               resolvedEnd,
				Dimensions:            splitCSV(dimensions),
				Type:                  typ,
				DimensionFilterGroups: groups,
				RowLimit:              rowLimit,
				StartRow:              startRow,
				DataState:             dataState,
				AggregationType:       aggregation,
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, s.base()+"/sites/"+escapePathSegment(site)+"/searchAnalytics/query", req)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderQueryRows(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&site, "site", "", "property URL-prefix or Domain property")
	f.StringVar(&start, "start", "", "start date YYYY-MM-DD (PT, inclusive)")
	f.StringVar(&end, "end", "", "end date YYYY-MM-DD (PT, inclusive)")
	f.IntVar(&days, "days", 0, "convenience: last N days (PT); mutually exclusive with --start/--end")
	f.StringVar(&dimensions, "dimensions", "", "comma-separated: query,page,country,device,date,hour,searchAppearance")
	f.StringVar(&typ, "type", "", "search type: web (default), image, video, news, discover, googleNews")
	f.StringArrayVar(&filters, "filter", nil, "repeatable dimension:operator:expression (one AND group)")
	f.IntVar(&rowLimit, "row-limit", 0, "rows to return, 1-25000 (API default 1000)")
	f.IntVar(&startRow, "start-row", 0, "zero-based row offset for pagination")
	f.StringVar(&dataState, "data-state", "", "final (default), all, or hourly_all")
	f.StringVar(&aggregation, "aggregation", "", "auto, byPage, byProperty, or byNewsShowcasePanel")
	return cmd
}

// resolveWindow enforces the --start/--end vs --days contract and returns the
// resolved YYYY-MM-DD window. --days N yields the inclusive N-day window ending
// on today (PT).
func (s *Service) resolveWindow(start, end string, days int) (string, string, error) {
	if days > 0 {
		if start != "" || end != "" {
			return "", "", &usageError{msg: "--days and --start/--end are mutually exclusive"}
		}
		loc, err := time.LoadLocation(ptZone)
		if err != nil {
			return "", "", &apiError{msg: fmt.Sprintf("search-console: load %s: %v", ptZone, err), err: err}
		}
		today := s.clock().In(loc)
		endDate := today.Format("2006-01-02")
		startDate := today.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
		return startDate, endDate, nil
	}
	if start == "" {
		return "", "", &usageError{msg: "--start (YYYY-MM-DD) is required (or use --days)"}
	}
	if end == "" {
		return "", "", &usageError{msg: "--end (YYYY-MM-DD) is required"}
	}
	return start, end, nil
}

// parseFilters converts repeatable dimension:operator:expression flags into a
// single AND group (the only groupType the API supports). Nil when none given.
func parseFilters(raw []string) ([]dimensionFilterGroup, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	filters := make([]dimensionFilter, 0, len(raw))
	for _, r := range raw {
		parts := strings.SplitN(r, ":", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
			return nil, &usageError{msg: fmt.Sprintf("--filter %q must be dimension:operator:expression", r)}
		}
		filters = append(filters, dimensionFilter{
			Dimension:  parts[0],
			Operator:   parts[1],
			Expression: parts[2],
		})
	}
	return []dimensionFilterGroup{{GroupType: "and", Filters: filters}}, nil
}

// splitCSV splits a comma-separated flag value, trimming blanks. Returns nil for
// an empty input so the field is omitted from the request.
func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// renderQueryRows prints the human-readable table: keys joined by " | " then
// clicks, impressions, ctr (as a percentage), and position.
func (s *Service) renderQueryRows(body []byte) error {
	var resp struct {
		Rows []struct {
			Keys        []string `json:"keys"`
			Clicks      float64  `json:"clicks"`
			Impressions float64  `json:"impressions"`
			CTR         float64  `json:"ctr"`
			Position    float64  `json:"position"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return &apiError{msg: fmt.Sprintf("search-console: decode query response: %v", err), err: err}
	}
	if len(resp.Rows) == 0 {
		fmt.Fprintln(s.stdout(), "no rows")
		return nil
	}
	fmt.Fprintf(s.stdout(), "%-40s\t%8s\t%12s\t%7s\t%8s\n", "keys", "clicks", "impressions", "ctr", "position")
	for _, r := range resp.Rows {
		fmt.Fprintf(s.stdout(), "%-40s\t%8.0f\t%12.0f\t%6.2f%%\t%8.2f\n",
			strings.Join(r.Keys, " | "), r.Clicks, r.Impressions, r.CTR*100, r.Position)
	}
	return nil
}
