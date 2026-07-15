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

// newViewCreateCmd is `view create` (POST /v1/views, markdownVersion). Exactly
// one of three mutually-exclusive parent flags is required: --database-id builds
// a view at a database's top level; --view-id adds a widget view to an existing
// dashboard; --create-database creates a linked database view on a page. The
// endpoint requires `name` and (for the --database-id / --view-id paths) a
// `data_source_id` — the view is built over a specific data source of the
// database (found via `fetch <db-id>` → data_sources[]). --type is required and
// passed through transparently — parent×type legality is left to the endpoint,
// not validated client-side. Output JSON.
func (s *Service) newViewCreateCmd(token string) *cobra.Command {
	var databaseID, viewID, createDatabaseFlag, dataSourceID, name, typ string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a view",
		Args:  cobra.NoArgs,
	}
	f := cmd.Flags()
	f.StringVar(&databaseID, "database-id", "", "create the view at this database's top level")
	f.StringVar(&viewID, "view-id", "", "add a widget view to this existing dashboard view")
	f.StringVar(&createDatabaseFlag, "create-database", "", "JSON create_database spec for a linked database view")
	f.StringVar(&dataSourceID, "data-source-id", "", "data source the view is built over (required with --database-id/--view-id)")
	f.StringVar(&name, "name", "", "view name (required)")
	f.StringVar(&typ, "type", "", "view type: "+strings.Join(viewTypes, "|")+" (required)")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")

	validateType := enumValidator("type", viewTypes...)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := validateType(typ); err != nil {
			return err
		}
		payload := map[string]any{"type": typ, "name": name}
		set := 0
		usesDataSource := false
		if cmd.Flags().Changed("database-id") {
			set++
			usesDataSource = true
			id, err := resolveID(databaseID)
			if err != nil {
				return err
			}
			payload["database_id"] = id
		}
		if cmd.Flags().Changed("view-id") {
			set++
			usesDataSource = true
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
		// POST /v1/views needs the specific data source the view is built over
		// for the --database-id / --view-id paths (a database may hold several).
		if usesDataSource && !cmd.Flags().Changed("data-source-id") {
			return &usageError{msg: "view create with --database-id/--view-id requires --data-source-id (run `fetch <db-id>` and use one of its data_sources[] ids)"}
		}
		if cmd.Flags().Changed("data-source-id") {
			id, err := resolveID(dataSourceID)
			if err != nil {
				return err
			}
			payload["data_source_id"] = id
		}
		body, err := s.callWithVersion(cmd.Context(), token, http.MethodPost, "/views", payload, markdownVersion)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newViewUpdateCmd is `view update <id>` (PATCH /v1/views/{id}, markdownVersion).
// --type is intentionally absent: a view's type is immutable after creation.
// --filters and --sorts pass through verbatim. Output JSON.
func (s *Service) newViewUpdateCmd(token string) *cobra.Command {
	var name, filtersFlag, sortsFlag string
	cmd := &cobra.Command{
		Use:   "update <view-id>",
		Short: "Update a view's name, filters, or sorts",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&name, "name", "", "new view name")
	cmd.Flags().StringVar(&filtersFlag, "filters", "", "JSON filters (REST wire)")
	cmd.Flags().StringVar(&sortsFlag, "sorts", "", "JSON sorts array (REST wire)")
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
		if len(payload) == 0 {
			return &usageError{msg: "view update requires at least one of --name, --filters, --sorts"}
		}
		body, err := s.callWithVersion(cmd.Context(), token, http.MethodPatch, "/views/"+url.PathEscape(id), payload, markdownVersion)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
