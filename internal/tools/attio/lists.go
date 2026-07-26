package attio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newListListCmd is `list list` (GET /v2/lists): discover pipeline/view lists.
func (s *Service) newListListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all lists",
		Long: "Lists are pipelines and views overlaid on records, each with its own\n" +
			"attributes. The slugs and ids here are what every `entry` command takes\n" +
			"and what `attribute list --list <slug>` describes. A record can sit on\n" +
			"several lists at once, and each membership is a separate entry with its\n" +
			"own id.",
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
		Use:   "get <list>",
		Short: "Get one list by slug or id",
		Long: "Takes a list slug or id and returns the list's own definition — name,\n" +
			"parent object, workspace access. Its entries are not included: query those\n" +
			"with `entry query <list>`, and its list-scoped attributes with `attribute\n" +
			"list --list <slug>`.",
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
		Use:   "query <list>",
		Short: "Query a list's entries with filter/sorts",
		Long: "The pageable read for a pipeline: --filter and --sorts are Attio-wire JSON\n" +
			"and --limit/--offset go in the request body. Filter keys are the LIST's\n" +
			"own attribute slugs from `attribute list --list <slug>`, not the parent\n" +
			"object's — a stage that lives on the list is not reachable through\n" +
			"`record query`.",
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
		Use:   "add <list> --parent-record <id> --parent-object <o>",
		Short: "Add a record to a list",
		Long: "--parent-record and --parent-object are both required, because a record id\n" +
			"only means something together with its object. --values is optional and\n" +
			"seeds the entry's LIST-scoped attributes — a pipeline stage, an owner on\n" +
			"this list — which are separate from the record's own attributes and are\n" +
			"discovered with `attribute list --list <slug>`.",
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
		Use:   "get <list> <entry_id>",
		Short: "Get one list entry by id",
		Long: "Both arguments are positional and ordered: the list first, then the entry\n" +
			"id. The entry carries its list-scoped values plus a pointer to the parent\n" +
			"record; the record's own attributes are not included and need `record get\n" +
			"<object> <record_id>`.",
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
		Use:   "remove <list> <entry_id>",
		Short: "Remove a list entry by id",
		Long: "Takes the record off this ONE list; the record itself, and its entries on\n" +
			"every other list, are untouched. This is what \"take it out of the\n" +
			"pipeline\" means — `record delete` removes the record everywhere and is\n" +
			"almost never the intent.",
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
		Use:   "update <list> <entry_id> --values <json>",
		Short: "Update a list entry (default overwrite; --append to append multiselect)",
		Long: "Same duality as `record update`: by default the values sent REPLACE the\n" +
			"entry's list-scoped values and a multiselect is reset to exactly what is\n" +
			"passed, while --append adds instead. It writes only the entry's\n" +
			"list-scoped attributes — changing a field on the underlying record is\n" +
			"`record update`.",
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
