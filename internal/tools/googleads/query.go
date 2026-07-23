package googleads

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// customerIDPattern matches a bare numeric Google Ads customer id after
// normalization (hyphens/spaces stripped). GAQL and the resource path both take
// digits only.
var customerIDPattern = regexp.MustCompile(`^[0-9]+$`)

// fieldTokenPattern matches a GAQL field token (resource.attribute, dotted,
// snake_case). It gates --metrics / --segments values so report can compose a
// GAQL SELECT list without opening a query-injection hole; anything more exotic
// belongs in the raw `query` escape hatch.
var fieldTokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// dateRangeLiterals is the closed set of GAQL DURING literals `report` accepts.
// Custom windows (BETWEEN 'a' AND 'b') are intentionally out of scope for the
// sugar command — use `query` with hand-written GAQL for those.
var dateRangeLiterals = map[string]struct{}{
	"TODAY": {}, "YESTERDAY": {}, "LAST_7_DAYS": {}, "LAST_14_DAYS": {},
	"LAST_30_DAYS": {}, "THIS_WEEK_SUN_TODAY": {}, "THIS_WEEK_MON_TODAY": {},
	"LAST_WEEK_SUN_SAT": {}, "LAST_WEEK_MON_SUN": {}, "THIS_MONTH": {},
	"LAST_MONTH": {}, "LAST_BUSINESS_WEEK": {}, "ALL_TIME": {},
}

// normalizeCustomerID strips the hyphens/spaces a user may copy from the Google
// Ads UI (123-456-7890) and validates the digits-only result.
func normalizeCustomerID(raw string) (string, error) {
	id := strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(raw))
	if !customerIDPattern.MatchString(id) {
		return "", &usageError{msg: fmt.Sprintf("--customer-id %q must be a numeric Google Ads customer id (digits, optional hyphens)", raw)}
	}
	return id, nil
}

// runSearch executes a GAQL query against one customer. When stream is true it
// uses googleAds:searchStream and flattens the JSON array of chunks into a
// single {results,fieldMask} object; otherwise it uses the paged
// googleAds:search and emits its single-object response verbatim (results +
// nextPageToken passthrough).
func (s *Service) runSearch(cmd *cobra.Command, c creds, customerID, gaql string, stream bool, pageSize int, pageToken string) error {
	lc, err := loginCustomerID(cmd)
	if err != nil {
		return err
	}
	c.loginCustomerID = lc
	if stream {
		payload := map[string]any{"query": gaql}
		body, err := s.call(cmd.Context(), c, http.MethodPost, "/customers/"+customerID+"/googleAds:searchStream", nil, payload)
		if err != nil {
			return err
		}
		flat, err := flattenStream(body)
		if err != nil {
			return err
		}
		return s.emitValue(flat)
	}
	payload := map[string]any{"query": gaql}
	if pageSize > 0 {
		payload["pageSize"] = pageSize
	}
	if pageToken != "" {
		payload["pageToken"] = pageToken
	}
	body, err := s.call(cmd.Context(), c, http.MethodPost, "/customers/"+customerID+"/googleAds:search", nil, payload)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// newQueryCmd is the raw GAQL escape hatch: full query power for the AI.
func (s *Service) newQueryCmd(c creds) *cobra.Command {
	var customerID, gaql, pageToken string
	var stream bool
	var pageSize int
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Run a raw GAQL query (POST googleAds:search / :searchStream)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := normalizeCustomerID(customerID)
			if err != nil {
				return err
			}
			if strings.TrimSpace(gaql) == "" {
				return &usageError{msg: "--gaql is required"}
			}
			return s.runSearch(cmd, c, id, gaql, stream, pageSize, pageToken)
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "target Google Ads customer id (required)")
	cmd.Flags().StringVar(&gaql, "gaql", "", "GAQL query string (required)")
	cmd.Flags().BoolVar(&stream, "stream", false, "use searchStream (single streamed response, no paging)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "max rows per page (search only; ignored with --stream)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a prior response (search only)")
	_ = cmd.MarkFlagRequired("customer-id")
	_ = cmd.MarkFlagRequired("gaql")
	return cmd
}

// reportResource maps a --resource value to its GAQL FROM clause and default
// SELECT dimension fields (metrics are appended separately).
type reportResource struct {
	from       string
	dimensions []string
}

var reportResources = map[string]reportResource{
	"campaign": {
		from:       "campaign",
		dimensions: []string{"campaign.id", "campaign.name", "campaign.status"},
	},
	"ad_group": {
		from:       "ad_group",
		dimensions: []string{"campaign.name", "ad_group.id", "ad_group.name", "ad_group.status"},
	},
	"keyword": {
		from:       "keyword_view",
		dimensions: []string{"campaign.name", "ad_group.name", "ad_group_criterion.keyword.text", "ad_group_criterion.keyword.match_type"},
	},
}

// defaultReportMetrics is the common performance metric set report selects when
// --metrics is omitted.
var defaultReportMetrics = []string{"metrics.impressions", "metrics.clicks", "metrics.cost_micros", "metrics.conversions"}

// newReportCmd is sugar over query: it composes a GAQL SELECT … FROM … WHERE
// segments.date DURING … from flags so the common "how did X perform" ask needs
// no hand-written GAQL, while `query` remains the escape hatch for anything
// else. It always uses searchStream (report pulls are bulk, paging-free).
func (s *Service) newReportCmd(c creds) *cobra.Command {
	var customerID, resource, dateRange, metricsCSV, segmentsCSV string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Build and run a GAQL performance report (POST googleAds:searchStream)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := normalizeCustomerID(customerID)
			if err != nil {
				return err
			}
			gaql, err := buildReportGAQL(resource, dateRange, metricsCSV, segmentsCSV)
			if err != nil {
				return err
			}
			return s.runSearch(cmd, c, id, gaql, true, 0, "")
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "target Google Ads customer id (required)")
	cmd.Flags().StringVar(&resource, "resource", "", "report entity: campaign|ad_group|keyword (required)")
	cmd.Flags().StringVar(&dateRange, "date-range", "LAST_30_DAYS", "GAQL DURING literal, e.g. LAST_30_DAYS, LAST_7_DAYS, THIS_MONTH")
	cmd.Flags().StringVar(&metricsCSV, "metrics", "", "comma-separated GAQL metric fields (default impressions,clicks,cost_micros,conversions)")
	cmd.Flags().StringVar(&segmentsCSV, "segments", "", "comma-separated GAQL segment fields (optional, e.g. segments.date)")
	_ = cmd.MarkFlagRequired("customer-id")
	_ = cmd.MarkFlagRequired("resource")
	return cmd
}

// buildReportGAQL composes the report GAQL. It validates the resource enum, the
// DURING literal, and every field token so no unvalidated input reaches the
// query string.
func buildReportGAQL(resource, dateRange, metricsCSV, segmentsCSV string) (string, error) {
	res, ok := reportResources[strings.TrimSpace(resource)]
	if !ok {
		return "", &usageError{msg: fmt.Sprintf("--resource %q is invalid (want campaign|ad_group|keyword)", resource)}
	}
	rangeLiteral := strings.ToUpper(strings.TrimSpace(dateRange))
	if _, ok := dateRangeLiterals[rangeLiteral]; !ok {
		return "", &usageError{msg: fmt.Sprintf("--date-range %q is not a supported GAQL DURING literal; use `query` for custom windows", dateRange)}
	}

	metrics := defaultReportMetrics
	if strings.TrimSpace(metricsCSV) != "" {
		parsed, err := parseFieldList("--metrics", metricsCSV)
		if err != nil {
			return "", err
		}
		metrics = parsed
	}
	segments, err := parseFieldList("--segments", segmentsCSV)
	if err != nil {
		return "", err
	}

	selectFields := make([]string, 0, len(res.dimensions)+len(metrics)+len(segments))
	selectFields = append(selectFields, res.dimensions...)
	selectFields = append(selectFields, segments...)
	selectFields = append(selectFields, metrics...)

	return fmt.Sprintf("SELECT %s FROM %s WHERE segments.date DURING %s",
		strings.Join(selectFields, ", "), res.from, rangeLiteral), nil
}

// parseFieldList splits a comma-separated field list and validates each token
// against fieldTokenPattern. An empty input yields an empty slice.
func parseFieldList(flag, csv string) ([]string, error) {
	if strings.TrimSpace(csv) == "" {
		return nil, nil
	}
	raw := strings.Split(csv, ",")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		token := strings.TrimSpace(r)
		if token == "" {
			continue
		}
		if !fieldTokenPattern.MatchString(token) {
			return nil, &usageError{msg: fmt.Sprintf("%s value %q is not a valid GAQL field (want dotted snake_case, e.g. metrics.clicks)", flag, token)}
		}
		out = append(out, token)
	}
	return out, nil
}

// intToString renders an int as a base-10 string (used by mutate builders).
func intToString(n int64) string { return strconv.FormatInt(n, 10) }
