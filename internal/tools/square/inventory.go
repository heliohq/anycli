package square

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newInventoryGetCmd(token string) *cobra.Command {
	var bodyJSON string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Batch-retrieve inventory counts (POST /v2/inventory/counts/batch-retrieve)",
		Args:  cobra.NoArgs,
		// Square models this read as a POST batch-retrieve; it never mutates stock.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/inventory/counts/batch-retrieve", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "BatchRetrieveInventoryCounts request body as raw JSON (catalog_object_ids, location_ids, cursor)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}
