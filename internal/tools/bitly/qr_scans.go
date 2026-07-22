package bitly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newQRScansCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "scans", Short: "Per-QR-code scan metrics"}
	cmd.AddCommand(
		s.newQRScanMetricCmd(token, "scans", "Scan counts over time (GET /qr-codes/{qrcode_id}/scans)", "/scans", false),
		s.newQRScanMetricCmd(token, "summary", "Total scan count (GET /qr-codes/{qrcode_id}/scans/summary)", "/scans/summary", false),
		s.newQRScanMetricCmd(token, "countries", "Scans by country (GET /qr-codes/{qrcode_id}/scans/countries)", "/scans/countries", true),
		s.newQRScanMetricCmd(token, "cities", "Scans by city (GET /qr-codes/{qrcode_id}/scans/cities)", "/scans/cities", true),
		s.newQRScanMetricCmd(token, "device-os", "Scans by device OS (GET /qr-codes/{qrcode_id}/scans/device_os)", "/scans/device_os", true),
		s.newQRScanMetricCmd(token, "browsers", "Scans by browser (GET /qr-codes/{qrcode_id}/scans/browsers)", "/scans/browsers", true),
	)
	return cmd
}

// newQRScanMetricCmd builds one per-QR-code scan metric command. suffix is the
// path tail appended to /qr-codes/{qrcode_id}; withSize adds --size for
// breakdown endpoints.
func (s *Service) newQRScanMetricCmd(token, use, short, suffix string, withSize bool) *cobra.Command {
	var qr string
	var analytics analyticsParams
	var size int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // all scan metrics are GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			analytics.apply(q)
			if withSize {
				q.Set("size", intToString(size))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/qr-codes/"+url.PathEscape(qr)+suffix, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&qr, "qr", "", "qrcode id")
	registerAnalyticsFlags(cmd, &analytics)
	if withSize {
		registerSizeFlag(cmd, &size)
	}
	_ = cmd.MarkFlagRequired("qr")
	return cmd
}
