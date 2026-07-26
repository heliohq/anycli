package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLocationListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List seller locations (GET /v2/locations)",
		Long: "Run this first. Each row's `id` is the `location_id` that the payment,\n" +
			"invoice, order and inventory calls take, and `status` separates an ACTIVE\n" +
			"location from a closed one that still owns historical data. It takes no flags\n" +
			"and is not paginated — a seller has few locations, and Square returns them all.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/locations", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newLocationGetCmd(token string) *cobra.Command {
	var locationID string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Retrieve a location (GET /v2/locations/{location_id})",
		Long: "`--location-id` is required but accepts the literal `main` as well as a real\n" +
			"id, which is the one place in this tool an id can be sidestepped. Returns the\n" +
			"address, timezone, capabilities and — worth reading — the location's currency,\n" +
			"since every money amount elsewhere is in that currency's minor units.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/locations/"+url.PathEscape(locationID), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&locationID, "location-id", "", "location id (or 'main' for the main location)")
	_ = cmd.MarkFlagRequired("location-id")
	return cmd
}
