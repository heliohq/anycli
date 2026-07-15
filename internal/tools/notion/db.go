package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newDBCreateCmd is `db create` (POST /v1/databases). It creates the database
// container. --properties is a data-source schema object that the CLI wraps
// into initial_data_source.properties — the 2026-03-11 data model no longer
// accepts a top-level `properties` field, and this is an intentional divergence
// from the MCP SQL-DDL `schema` param (design 304 §db/data-source). Output JSON.
func (s *Service) newDBCreateCmd(token string) *cobra.Command {
	var parent, title, propertiesFlag string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a database container",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&parent, "parent", "", "parent page: JSON, URL, or id (required)")
	cmd.Flags().StringVar(&title, "title", "", "database title")
	cmd.Flags().StringVar(&propertiesFlag, "properties", "", "JSON data-source schema (wrapped into initial_data_source.properties)")
	_ = cmd.MarkFlagRequired("parent")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		parentWire, err := s.parentWire(cmd.Context(), token, parent)
		if err != nil {
			return err
		}
		properties, err := parseJSONFlag("properties", propertiesFlag)
		if err != nil {
			return err
		}
		payload := map[string]any{"parent": parentWire}
		if strings.TrimSpace(title) != "" {
			// POST /v1/databases wants title as a rich-text array, not a bare
			// string; wrap the scalar into a single text run.
			payload["title"] = []any{
				map[string]any{"type": "text", "text": map[string]any{"content": title}},
			}
		}
		if properties != nil {
			payload["initial_data_source"] = map[string]any{"properties": properties}
		}
		body, err := s.callWithVersion(cmd.Context(), token, http.MethodPost, "/databases", payload, markdownVersion)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newDBQueryCmd is `db query <id>` (POST /v1/data_sources/{id}/query). The id is
// a data-source id only; a database id is rejected fail-fast (its records live
// in a data source, found via `fetch <db-id>` → data_sources[]). Standard
// filter/sorts — not the AI-only SQL query. Paginated. Output JSON.
func (s *Service) newDBQueryCmd(token string) *cobra.Command {
	var filterFlag, sortsFlag string
	cmd := &cobra.Command{
		Use:   "query <data-source-id>",
		Short: "Query a data source with filter/sorts",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&filterFlag, "filter", "", "JSON filter (REST wire)")
	cmd.Flags().StringVar(&sortsFlag, "sorts", "", "JSON sorts array (REST wire)")
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		filter, err := parseJSONFlag("filter", filterFlag)
		if err != nil {
			return err
		}
		sorts, err := parseJSONFlag("sorts", sortsFlag)
		if err != nil {
			return err
		}
		if err := s.rejectDatabaseID(cmd.Context(), token, id, "db query"); err != nil {
			return err
		}
		fetch := func(ctx context.Context, cursor string) ([]byte, error) {
			payload := map[string]any{}
			if filter != nil {
				payload["filter"] = filter
			}
			if sorts != nil {
				payload["sorts"] = sorts
			}
			if pf.pageSize > 0 {
				payload["page_size"] = pf.pageSize
			}
			if cursor != "" {
				payload["start_cursor"] = cursor
			}
			return s.callWithVersion(ctx, token, http.MethodPost, "/data_sources/"+url.PathEscape(id)+"/query", payload, markdownVersion)
		}
		body, err := paginate(cmd.Context(), pf.all, pf.startCursor, fetch)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newDataSourceUpdateCmd is `data-source update <id>` (PATCH /v1/data_sources/{id}).
// The id is a data-source id only (same database-id fail-fast as db query).
// --properties is a schema patch passed through verbatim. Output JSON.
func (s *Service) newDataSourceUpdateCmd(token string) *cobra.Command {
	var propertiesFlag, name, description string
	cmd := &cobra.Command{
		Use:   "update <data-source-id>",
		Short: "Update a data source's schema, name, or description",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&propertiesFlag, "properties", "", "JSON schema patch (REST wire)")
	cmd.Flags().StringVar(&name, "name", "", "new data-source name")
	cmd.Flags().StringVar(&description, "description", "", "new data-source description")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		properties, err := parseJSONFlag("properties", propertiesFlag)
		if err != nil {
			return err
		}
		payload := map[string]any{}
		if properties != nil {
			payload["properties"] = properties
		}
		if cmd.Flags().Changed("name") {
			payload["name"] = name
		}
		if cmd.Flags().Changed("description") {
			payload["description"] = description
		}
		if len(payload) == 0 {
			return &usageError{msg: "data-source update requires at least one of --properties, --name, --description"}
		}
		if err := s.rejectDatabaseID(cmd.Context(), token, id, "data-source update"); err != nil {
			return err
		}
		body, err := s.callWithVersion(cmd.Context(), token, http.MethodPatch, "/data_sources/"+url.PathEscape(id), payload, markdownVersion)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// rejectDatabaseID fails fast when id is a database container. db query and
// data-source update accept data-source ids only; a positive GET
// /v1/databases/{id} confirms a database and points the caller at fetch →
// data_sources[]. Any non-2xx means "not a database" and the caller proceeds
// (the real data-source endpoint validates the id), so a data-source id that
// cannot be positively probed is never wrongly blocked.
func (s *Service) rejectDatabaseID(ctx context.Context, token, id, cmd string) error {
	if _, err := s.callWithVersion(ctx, token, http.MethodGet, "/databases/"+url.PathEscape(id), nil, markdownVersion); err == nil {
		return &usageError{msg: fmt.Sprintf(
			"%s only accepts a data-source id; %s is a database — run `fetch %s` and use one of its data_sources[] ids",
			cmd, id, id)}
	}
	return nil
}
