package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLocationListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List seller locations (GET /v2/locations)",
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
		Use:         "get",
		Short:       "Retrieve a location (GET /v2/locations/{location_id})",
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
