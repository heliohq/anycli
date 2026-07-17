package bitly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newAnalyticsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "analytics", Short: "Per-bitlink click metrics"}
	cmd.AddCommand(
		s.newBitlinkMetricCmd(token, "clicks", "Click counts over time (GET /bitlinks/{bitlink}/clicks)", "/clicks", false),
		s.newBitlinkMetricCmd(token, "clicks-summary", "Total click count (GET /bitlinks/{bitlink}/clicks/summary)", "/clicks/summary", false),
		s.newBitlinkMetricCmd(token, "countries", "Clicks by country (GET /bitlinks/{bitlink}/countries)", "/countries", true),
		s.newBitlinkMetricCmd(token, "cities", "Clicks by city (GET /bitlinks/{bitlink}/cities)", "/cities", true),
		s.newBitlinkMetricCmd(token, "devices", "Clicks by device (GET /bitlinks/{bitlink}/devices)", "/devices", true),
		s.newBitlinkMetricCmd(token, "referrers", "Clicks by referrer (GET /bitlinks/{bitlink}/referrers)", "/referrers", true),
		s.newBitlinkMetricCmd(token, "referrer-name", "Clicks by referrer name (GET /bitlinks/{bitlink}/referrer_name)", "/referrer_name", true),
		s.newBitlinkMetricCmd(token, "referrers-by-domains", "Referrers grouped by domain (GET /bitlinks/{bitlink}/referrers_by_domains)", "/referrers_by_domains", true),
		s.newBitlinkMetricCmd(token, "referring-domains", "Clicks by referring domain (GET /bitlinks/{bitlink}/referring_domains)", "/referring_domains", true),
		s.newBitlinkMetricCmd(token, "engagements", "Engagements over time (GET /bitlinks/{bitlink}/engagements)", "/engagements", false),
		s.newBitlinkMetricCmd(token, "engagements-summary", "Engagement totals (GET /bitlinks/{bitlink}/engagements/summary)", "/engagements/summary", false),
	)
	return cmd
}

// newBitlinkMetricCmd builds one per-bitlink analytics command. suffix is the
// path tail appended to /bitlinks/{bitlink}; withSize adds the --size flag for
// breakdown endpoints. The {bitlink} segment is sent verbatim (literal slash).
func (s *Service) newBitlinkMetricCmd(token, use, short, suffix string, withSize bool) *cobra.Command {
	var bitlink string
	var analytics analyticsParams
	var size int
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			analytics.apply(q)
			if withSize {
				q.Set("size", intToString(size))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/bitlinks/"+bitlink+suffix, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bitlink, "bitlink", "", "bitlink id, e.g. bit.ly/2ab (literal slash, not encoded)")
	registerAnalyticsFlags(cmd, &analytics)
	if withSize {
		registerSizeFlag(cmd, &size)
	}
	_ = cmd.MarkFlagRequired("bitlink")
	return cmd
}
