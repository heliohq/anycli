package attio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newListListCmd is `list list` (GET /v2/lists): discover pipeline/view lists.
func (s *Service) newListListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List all lists",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/lists", nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newListGetCmd is `list get <list>` (GET /v2/lists/{list}).
func (s *Service) newListGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <list>",
		Short:       "Get one list by slug or id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/lists/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newEntryQueryCmd is `entry query <list>` (POST /v2/lists/{list}/entries/query):
// query a pipeline's entries with filter/sorts; limit/offset in the body.
func (s *Service) newEntryQueryCmd(token string) *cobra.Command {
	var filterFlag, sortsFlag string
	cmd := &cobra.Command{
		Use:         "query <list>",
		Short:       "Query a list's entries with filter/sorts",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&filterFlag, "filter", "", "JSON filter object (Attio wire)")
	cmd.Flags().StringVar(&sortsFlag, "sorts", "", "JSON sorts array (Attio wire)")
	lo := registerLimitOffset(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		filter, err := parseJSONFlag("filter", filterFlag)
		if err != nil {
			return err
		}
		sorts, err := parseJSONFlag("sorts", sortsFlag)
		if err != nil {
			return err
		}
		payload := map[string]any{}
		if filter != nil {
			payload["filter"] = filter
		}
		if sorts != nil {
			payload["sorts"] = sorts
		}
		lo.applyToPayload(payload)
		path := "/v2/lists/" + url.PathEscape(args[0]) + "/entries/query"
		body, err := s.call(cmd.Context(), token, http.MethodPost, path, payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newEntryAddCmd is `entry add <list> --parent-record <id> --parent-object <o>`
// (POST /v2/lists/{list}/entries): add a record to a list. Optional --values
// seeds list-scoped entry_values.
func (s *Service) newEntryAddCmd(token string) *cobra.Command {
	var parentRecord, parentObject, valuesFlag string
	cmd := &cobra.Command{
		Use:         "add <list> --parent-record <id> --parent-object <o>",
		Short:       "Add a record to a list",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&parentRecord, "parent-record", "", "id of the record to add (required)")
	cmd.Flags().StringVar(&parentObject, "parent-object", "", "object slug/id the record belongs to (required)")
	cmd.Flags().StringVar(&valuesFlag, "values", "", "optional JSON object of list-scoped attribute slug/id → value")
	_ = cmd.MarkFlagRequired("parent-record")
	_ = cmd.MarkFlagRequired("parent-object")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		data := map[string]any{
			"parent_record_id": parentRecord,
			"parent_object":    parentObject,
			"entry_values":     map[string]any{},
		}
		if cmd.Flags().Changed("values") {
			values, err := parseValuesFlag(valuesFlag)
			if err != nil {
				return err
			}
			data["entry_values"] = values
		}
		payload := map[string]any{"data": data}
		path := "/v2/lists/" + url.PathEscape(args[0]) + "/entries"
		body, err := s.call(cmd.Context(), token, http.MethodPost, path, payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newEntryGetCmd is `entry get <list> <entry_id>`
// (GET /v2/lists/{list}/entries/{entry_id}).
func (s *Service) newEntryGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <list> <entry_id>",
		Short:       "Get one list entry by id",
		Args:        cobra.ExactArgs(2),
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, entryPath(args[0], args[1]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newEntryRemoveCmd is `entry remove <list> <entry_id>`
// (DELETE /v2/lists/{list}/entries/{entry_id}): remove a record from a list.
func (s *Service) newEntryRemoveCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "remove <list> <entry_id>",
		Short:       "Remove a list entry by id",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodDelete, entryPath(args[0], args[1]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newEntryUpdateCmd is `entry update <list> <entry_id> --values <json>`. Same
// duality as record update: default PUT (overwrite/remove multiselect),
// --append switches to PATCH (append). Carries the data.entry_values envelope.
func (s *Service) newEntryUpdateCmd(token string) *cobra.Command {
	var valuesFlag string
	var appendMode bool
	cmd := &cobra.Command{
		Use:         "update <list> <entry_id> --values <json>",
		Short:       "Update a list entry (default overwrite; --append to append multiselect)",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&valuesFlag, "values", "", "JSON object of attribute slug/id → value (required)")
	cmd.Flags().BoolVar(&appendMode, "append", false, "append multiselect values (PATCH) instead of overwriting (PUT)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseValuesFlag(valuesFlag)
		if err != nil {
			return err
		}
		payload := map[string]any{"data": map[string]any{"entry_values": values}}
		method := http.MethodPut
		if appendMode {
			method = http.MethodPatch
		}
		body, err := s.call(cmd.Context(), token, method, entryPath(args[0], args[1]), payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// entryPath builds /v2/lists/{list}/entries/{entry_id}.
func entryPath(list, entryID string) string {
	return "/v2/lists/" + url.PathEscape(list) + "/entries/" + url.PathEscape(entryID)
}
