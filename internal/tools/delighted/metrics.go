package delighted

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMetricsCmd wires `delighted metrics get` — GET /metrics.json, the NPS/CSAT/
// CES aggregate scores over an optional time window.
func (s *Service) newMetricsCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "metrics", Short: "NPS/CSAT/CES aggregate scores"}

	var since, until string
	var trend string
	get := &cobra.Command{
		Use:         "get",
		Short:       "Read aggregate metrics (GET /metrics.json)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIfNonEmpty(q, "since", since)
			setIfNonEmpty(q, "until", until)
			setIfNonEmpty(q, "trend", trend)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/metrics.json", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	get.Flags().StringVar(&since, "since", "", "start of the window (Unix timestamp)")
	get.Flags().StringVar(&until, "until", "", "end of the window (Unix timestamp)")
	get.Flags().StringVar(&trend, "trend", "", "trend id to scope the metrics to")

	cmd.AddCommand(get)
	return cmd
}
