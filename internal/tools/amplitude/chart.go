package amplitude

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// chartEnvelope wraps the CSV a saved chart returns so stdout stays JSON.
type chartEnvelope struct {
	Format  string `json:"format"`
	ChartID string `json:"chart_id"`
	Data    string `json:"data"`
}

// newChartCmd — GET /api/3/chart/:id/csv. Reads the results behind an existing
// saved chart (an analyst already built it in the UI). The response is CSV, so
// it is wrapped in a JSON envelope rather than emitted raw.
func (s *Service) newChartCmd(authHeader string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "chart",
		Short: "Results behind a saved chart as CSV (GET /api/3/chart/:id/csv)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required (a saved chart id)"}
			}
			inv, err := s.resolve(cmd, authHeader)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), inv, http.MethodGet, "/api/3/chart/"+url.PathEscape(id)+"/csv", nil)
			if err != nil {
				return err
			}
			return s.emitValue(chartEnvelope{Format: "csv", ChartID: id, Data: string(body)})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "saved chart id (required)")
	return cmd
}
