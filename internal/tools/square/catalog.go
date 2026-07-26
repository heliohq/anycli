package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCatalogListCmd(token string) *cobra.Command {
	var types, cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List catalog objects (GET /v2/catalog/list)",
		Long: "`--types` is a comma-separated filter (`ITEM`, `ITEM_VARIATION`, `CATEGORY`,\n" +
			"`TAX`, `DISCOUNT`, …) and is worth setting, because unfiltered this interleaves\n" +
			"every object type. A sellable thing is an ITEM with one or more\n" +
			"ITEM_VARIATION children, and it is the VARIATION id that inventory counts and\n" +
			"order line items reference — not the item's. Page with `--cursor`.",
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
		Long: "POST, but a read. `--body` is required and carries SearchCatalogObjects:\n" +
			"`object_types` plus a `query`, which is `text_query` for keyword matching or\n" +
			"`exact_query` for an attribute match. Prefer it to paging `catalog list` when\n" +
			"the catalog is large or the match is by name. It returns the matching objects\n" +
			"alone, without their parents or children.",
		Args: cobra.NoArgs,
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
		Use:   "get",
		Short: "Retrieve a catalog object (GET /v2/catalog/object/{object_id})",
		Long: "`--object-id` is required. `--include-related` also returns the objects this\n" +
			"one references — for an ITEM_VARIATION that is its parent ITEM, for an ITEM its\n" +
			"category and taxes. That is usually necessary to make sense of a variation id\n" +
			"picked up from an inventory count or an order line, which carries no name of\n" +
			"its own.",
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
