package notion

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// viewTypes is the closed --type enum for `view create` (design 304 §view).
// This set is distinct from fetch/search --type and gets its own validator.
var viewTypes = []string{
	"table", "board", "list", "calendar", "timeline",
	"gallery", "form", "chart", "map", "dashboard",
}

// newViewCreateCmd is `view create` (POST /v1/views). Exactly
// one of three mutually-exclusive parent flags is required: --database-id builds
// a view at a database's top level; --view-id adds a widget view to an existing
// dashboard; --create-database creates a linked database view on a page. The
// endpoint requires `name` and (for the --database-id / --view-id paths) a
// `data_source_id` — the view is built over a specific data source of the
// database (found via `fetch <db-id>` → data_sources[]). --type is required and
// passed through transparently — parent×type legality is left to the endpoint,
// not validated client-side. Output JSON.
func (s *Service) newViewCreateCmd(token string) *cobra.Command {
	var databaseID, viewID, createDatabaseFlag, dataSourceID, name, typ, configFlag, quickFiltersFlag string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a view",
		Args:  cobra.NoArgs,
	}
	f := cmd.Flags()
	f.StringVar(&databaseID, "database-id", "", "create the view at this database's top level")
	f.StringVar(&viewID, "view-id", "", "add a widget view to this existing dashboard view")
	f.StringVar(&createDatabaseFlag, "create-database", "", "JSON create_database spec for a linked database view")
	f.StringVar(&dataSourceID, "data-source-id", "", "data source the view is built over (required)")
	f.StringVar(&name, "name", "", "view name (required)")
	f.StringVar(&typ, "type", "", "view type: "+strings.Join(viewTypes, "|")+" (required)")
	f.StringVar(&configFlag, "configuration", "", "JSON view configuration (grouping, visible properties, layout; REST wire)")
	f.StringVar(&quickFiltersFlag, "quick-filters", "", "JSON quick_filters (REST wire)")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")
	// data_source_id is required for every parent mode — POST /v1/views rejects
	// a create without it (verified live, incl. the --create-database path).
	_ = cmd.MarkFlagRequired("data-source-id")

	validateType := enumValidator("type", viewTypes...)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := validateType(typ); err != nil {
			return err
		}
		dsID, err := resolveID(dataSourceID)
		if err != nil {
			return err
		}
		payload := map[string]any{"type": typ, "name": name, "data_source_id": dsID}
		set := 0
		if cmd.Flags().Changed("database-id") {
			set++
			id, err := resolveID(databaseID)
			if err != nil {
				return err
			}
			payload["database_id"] = id
		}
		if cmd.Flags().Changed("view-id") {
			set++
			id, err := resolveID(viewID)
			if err != nil {
				return err
			}
			payload["view_id"] = id
		}
		if cmd.Flags().Changed("create-database") {
			set++
			cd, err := parseJSONFlag("create-database", createDatabaseFlag)
			if err != nil {
				return err
			}
			payload["create_database"] = cd
		}
		if set != 1 {
			return &usageError{msg: "view create requires exactly one of --database-id, --view-id, --create-database"}
		}
		if config, err := parseJSONFlag("configuration", configFlag); err != nil {
			return err
		} else if config != nil {
			payload["configuration"] = config
		}
		if qf, err := parseJSONFlag("quick-filters", quickFiltersFlag); err != nil {
			return err
		} else if qf != nil {
			payload["quick_filters"] = qf
		}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/views", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newViewUpdateCmd is `view update <id>` (PATCH /v1/views/{id}).
// --type is intentionally absent: a view's type is immutable after creation.
// --filters and --sorts pass through verbatim. Output JSON.
func (s *Service) newViewUpdateCmd(token string) *cobra.Command {
	var name, filtersFlag, sortsFlag, configFlag, quickFiltersFlag string
	cmd := &cobra.Command{
		Use:   "update <view-id>",
		Short: "Update a view's name, filters, sorts, configuration, or quick filters",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&name, "name", "", "new view name")
	cmd.Flags().StringVar(&filtersFlag, "filters", "", "JSON filters (REST wire)")
	cmd.Flags().StringVar(&sortsFlag, "sorts", "", "JSON sorts array (REST wire)")
	cmd.Flags().StringVar(&configFlag, "configuration", "", "JSON view configuration (grouping, visible properties, layout; REST wire)")
	cmd.Flags().StringVar(&quickFiltersFlag, "quick-filters", "", "JSON quick_filters (REST wire)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		filters, err := parseJSONFlag("filters", filtersFlag)
		if err != nil {
			return err
		}
		sorts, err := parseJSONFlag("sorts", sortsFlag)
		if err != nil {
			return err
		}
		config, err := parseJSONFlag("configuration", configFlag)
		if err != nil {
			return err
		}
		quickFilters, err := parseJSONFlag("quick-filters", quickFiltersFlag)
		if err != nil {
			return err
		}
		payload := map[string]any{}
		if cmd.Flags().Changed("name") {
			payload["name"] = name
		}
		if filters != nil {
			payload["filters"] = filters
		}
		if sorts != nil {
			payload["sorts"] = sorts
		}
		if config != nil {
			payload["configuration"] = config
		}
		if quickFilters != nil {
			payload["quick_filters"] = quickFilters
		}
		if len(payload) == 0 {
			return &usageError{msg: "view update requires at least one of --name, --filters, --sorts, --configuration, --quick-filters"}
		}
		body, err := s.call(cmd.Context(), token, http.MethodPatch, "/views/"+url.PathEscape(id), payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
