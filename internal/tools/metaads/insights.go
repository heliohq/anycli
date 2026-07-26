package metaads

import (
	"net/url"

	"github.com/spf13/cobra"
)

const defaultInsightsFields = "impressions,clicks,spend,reach,cpm,cpc,ctr,frequency,actions,date_start,date_stop"

// newInsightsCmd is the reporting command — what "how are my ads doing"
// resolves to. It reports either a whole account (--account) or one object
// (--object: campaign / ad set / ad), aggregated at --level, over a preset or
// explicit time range.
func (s *Service) newInsightsCmd(token string) *cobra.Command {
	var account, object, level, datePreset, timeRange, fields string
	var limit int
	cmd := &cobra.Command{
		Use:   "insights",
		Short: "Ad performance insights (GET /<account|object>/insights)",
		Long: "Exactly one of --account act_<id> or --object <numeric id> is required;\n" +
			"--object accepts a campaign, ad set or ad id. --level sets the row\n" +
			"granularity (account, campaign, adset, ad) and is validated locally.\n" +
			"\n" +
			"--date-preset and --time-range are mutually exclusive — Graph rejects both\n" +
			"— and with neither set the window is last_30d, so an unqualified insights\n" +
			"call is a 30-day report, not all-time. --time-range is explicit JSON\n" +
			"carrying `since` and `until` dates. --limit is 1-500, default 100. The\n" +
			"default fields cover impressions, clicks, spend, reach, CPM, CPC, CTR,\n" +
			"frequency and actions; conversions are inside `actions` rather than a\n" +
			"top-level field.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireExactlyOne("--account", account, "--object", object); err != nil {
				return err
			}
			node := object
			if account != "" {
				if err := requireAccountID(account); err != nil {
					return err
				}
				node = account
			} else if err := requireObjectID("--object", object); err != nil {
				return err
			}
			if err := requireInsightsLevel(level); err != nil {
				return err
			}
			if err := requireAtMostOne("--date-preset", datePreset, "--time-range", timeRange); err != nil {
				return err
			}
			if err := requireLimit(limit, 1, 500); err != nil {
				return err
			}

			query := url.Values{
				"fields": {fields},
				"limit":  {itoa(limit)},
			}
			if level != "" {
				query.Set("level", level)
			}
			// time_range is an explicit window and takes precedence; otherwise
			// fall back to the named preset. Never send both (Graph rejects it).
			if timeRange != "" {
				query.Set("time_range", timeRange)
			} else {
				preset := datePreset
				if preset == "" {
					preset = "last_30d"
				}
				query.Set("date_preset", preset)
			}

			body, err := s.get(cmd.Context(), token, "/"+node+"/insights", query)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "ad account id in act_<number> form (exactly one of --account/--object)")
	cmd.Flags().StringVar(&object, "object", "", "campaign, ad set, or ad id (exactly one of --account/--object)")
	cmd.Flags().StringVar(&level, "level", "", "aggregation level: account, campaign, adset, ad")
	cmd.Flags().StringVar(&datePreset, "date-preset", "", "named window, e.g. today, last_7d, last_30d (default last_30d)")
	cmd.Flags().StringVar(&timeRange, "time-range", "", `explicit window JSON, e.g. {"since":"2026-01-01","until":"2026-01-31"}`)
	cmd.Flags().StringVar(&fields, "fields", defaultInsightsFields, "comma-separated insights fields")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum rows in this page (1-500)")
	return cmd
}
