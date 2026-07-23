package onesignal

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newSegmentCreateCmd(key, appID string) *cobra.Command {
	var name, filters string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create an audience segment (POST /apps/{app_id}/segments)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return &usageError{msg: "--name is required"}
			}
			if filters == "" {
				return &usageError{msg: "--filters is required (a JSON array of OneSignal filters)"}
			}
			decoded, err := decodeJSONFlag("filters", filters)
			if err != nil {
				return err
			}
			body := map[string]any{"name": name, "filters": decoded}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, appPath(appID, "/segments"), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "segment name (required)")
	cmd.Flags().StringVar(&filters, "filters", "", "JSON array of OneSignal filters (required)")
	return cmd
}

func (s *Service) newSegmentListCmd(key, appID string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List segments (GET /apps/{app_id}/segments)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, appPath(appID, "/segments"), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newSegmentDeleteCmd(key, appID string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a segment (DELETE /apps/{app_id}/segments/{id})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, appPath(appID, "/segments/"+url.PathEscape(id)), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "segment id (required)")
	return cmd
}
