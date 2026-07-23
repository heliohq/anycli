package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCatalogListCmd(token string) *cobra.Command {
	var types, cursor string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List catalog objects (GET /v2/catalog/list)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setNonEmpty(q, "types", types)
			setNonEmpty(q, "cursor", cursor)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/catalog/list", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&types, "types", "", "comma-separated object types (ITEM, ITEM_VARIATION, CATEGORY, …)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	return cmd
}

func (s *Service) newCatalogSearchCmd(token string) *cobra.Command {
	var bodyJSON string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search catalog objects (POST /v2/catalog/search)",
		Args:  cobra.NoArgs,
		// POST /v2/catalog/search is a documented lookup; never mutates.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/catalog/search", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "SearchCatalogObjects request body as raw JSON (object_types, query, limit, cursor)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newCatalogGetCmd(token string) *cobra.Command {
	var objectID string
	var includeRelated bool
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Retrieve a catalog object (GET /v2/catalog/object/{object_id})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if includeRelated {
				q.Set("include_related_objects", "true")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/catalog/object/"+url.PathEscape(objectID), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&objectID, "object-id", "", "catalog object id")
	cmd.Flags().BoolVar(&includeRelated, "include-related", false, "include related objects")
	_ = cmd.MarkFlagRequired("object-id")
	return cmd
}
