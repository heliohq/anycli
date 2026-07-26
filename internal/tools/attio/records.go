package attio

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// defaultSearchObjects is the search default: the two standard objects present
// in every workspace. `deals`, `users` and `workspaces` are optional standard
// objects disabled until an admin activates them (and a Free-plan workspace caps
// at 3 objects total), and the search endpoint validates each slug — so
// defaulting to the full five-object set would make the natural first call
// `record search --query "…"` (no --objects) error with value_not_found on any
// default-onboarded or Free-plan workspace. Broader/custom search is opt-in via
// explicit --objects (slugs discoverable via `object list`).
var defaultSearchObjects = []string{"people", "companies"}

// newRecordSearchCmd is `record search --query <q>` (POST /v2/objects/records/search):
// fuzzy cross-object find. The body requires query, objects and request_as;
// --objects defaults to people,companies and request_as defaults to
// {"type":"workspace"}. --request-as-member scopes visibility to one member
// (a UUID becomes workspace_member_id, anything else email_address). limit only
// (default/max 25); no offset on this endpoint.
func (s *Service) newRecordSearchCmd(token string) *cobra.Command {
	var query, objectsFlag, requestAsMember string
	var limit int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Fuzzy-search records across objects",
		Long: "Defaults to `people,companies` ONLY — the two objects every workspace has.\n" +
			"Reaching deals, users, workspaces or a custom object means naming them in\n" +
			"--objects, and a slug that is not enabled in this workspace comes back as\n" +
			"a 400 value_not_found rather than an empty result, so take slugs from\n" +
			"`object list`.\n" +
			"\n" +
			"--query is required and capped at 256 characters. --limit both defaults to\n" +
			"and maxes out at 25 and this endpoint has NO offset, so results cannot be\n" +
			"paged past 25 — switch to `record query` with a filter when more is\n" +
			"needed. --request-as-member scopes results to what one member can see and\n" +
			"accepts either a member UUID or an email address.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&query, "query", "", "search query (required, max 256 chars)")
	cmd.Flags().StringVar(&objectsFlag, "objects", "", "comma-separated object slugs/ids (default: people,companies)")
	cmd.Flags().StringVar(&requestAsMember, "request-as-member", "", "scope results to a workspace member (id or email)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of results (default/max 25)")
	_ = cmd.MarkFlagRequired("query")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		objects := defaultSearchObjects
		if strings.TrimSpace(objectsFlag) != "" {
			objects = splitCSV(objectsFlag)
		}
		payload := map[string]any{
			"query":      query,
			"objects":    objects,
			"request_as": requestAs(requestAsMember),
		}
		if cmd.Flags().Changed("limit") {
			payload["limit"] = limit
		}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/objects/records/search", payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// requestAs builds the search request_as object. Empty member → the full
// workspace view; a member value that parses as a UUID → workspace_member_id,
// otherwise it is treated as an email address.
func requestAs(member string) map[string]any {
	member = strings.TrimSpace(member)
	if member == "" {
		return map[string]any{"type": "workspace"}
	}
	if isUUID(member) {
		return map[string]any{"type": "workspace-member", "workspace_member_id": member}
	}
	return map[string]any{"type": "workspace-member", "email_address": member}
}

// isUUID reports whether s has the canonical 8-4-4-4-12 hex UUID shape. Attio
// workspace-member ids are UUIDs; anything else is treated as an email address.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// splitCSV splits a comma-separated flag value, trimming spaces and dropping
// empties.
func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// newRecordQueryCmd is `record query <object>` (POST /v2/objects/{object}/records/query):
// exact filter/sort querying of one object. filter/sorts are raw JSON passthrough
// (the schema is per-workspace); limit/offset go in the body.
func (s *Service) newRecordQueryCmd(token string) *cobra.Command {
	var filterFlag, sortsFlag string
	cmd := &cobra.Command{
		Use:   "query <object>",
		Short: "Query one object's records with filter/sorts",
		Long: "The exact counterpart to `record search`: one object, --filter and --sorts\n" +
			"passed through verbatim as Attio-wire JSON, and --limit/--offset in the\n" +
			"request BODY — which is what makes this the pageable read. Filter keys are\n" +
			"attribute slugs from `attribute list --object <slug>` and the filter\n" +
			"follows Attio's operator syntax, not a search string. A list's own\n" +
			"attributes are not visible here; query those with `entry query`.",
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
		path := "/v2/objects/" + url.PathEscape(args[0]) + "/records/query"
		body, err := s.call(cmd.Context(), token, http.MethodPost, path, payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newRecordGetCmd is `record get <object> <record_id>`
// (GET /v2/objects/{object}/records/{record_id}).
func (s *Service) newRecordGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <object> <record_id>",
		Short: "Get one record by id",
		Long: "Both arguments are positional and ordered: the object slug first, then the\n" +
			"record id. A record id alone addresses nothing, because the object is part\n" +
			"of the path. The response carries every attribute as an array of typed\n" +
			"value objects — the same shape a --values payload has to produce.",
		Args:        cobra.ExactArgs(2),
		Annotations: readOnly,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, recordPath(args[0], args[1]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newRecordDeleteCmd is `record delete <object> <record_id>`
// (DELETE /v2/objects/{object}/records/{record_id}).
func (s *Service) newRecordDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <object> <record_id>",
		Short: "Delete one record by id",
		Long: "Takes the object slug and the record id positionally. The record vanishes\n" +
			"from every list it was on and nothing in this tool restores it. Taking a\n" +
			"record off ONE pipeline is `entry remove`, which leaves the record itself\n" +
			"intact — that is almost always what was actually meant.",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodDelete, recordPath(args[0], args[1]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newRecordCreateCmd is `record create <object> --values <json>`
// (POST /v2/objects/{object}/records) with the data.values envelope.
func (s *Service) newRecordCreateCmd(token string) *cobra.Command {
	var valuesFlag string
	cmd := &cobra.Command{
		Use:   "create <object> --values <json>",
		Short: "Create a record",
		Long: "--values is a JSON object of attribute slug to value and is wrapped into\n" +
			"the `data.values` envelope for you. Slugs come from `attribute list\n" +
			"--object <slug>`, and select or status values have to be options that\n" +
			"already exist — `attribute options` and `attribute statuses` list them.\n" +
			"Nothing deduplicates here, so use `record upsert --match` whenever the\n" +
			"record may already be in the workspace.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&valuesFlag, "values", "", "JSON object of attribute slug/id → value (required)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseValuesFlag(valuesFlag)
		if err != nil {
			return err
		}
		payload := map[string]any{"data": map[string]any{"values": values}}
		path := "/v2/objects/" + url.PathEscape(args[0]) + "/records"
		body, err := s.call(cmd.Context(), token, http.MethodPost, path, payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newRecordUpdateCmd is `record update <object> <record_id> --values <json>`.
// The default verb is PUT (overwrite/remove multiselect values — the intuitive
// "update replaces what I set"); --append switches to PATCH (append/prepend
// multiselect values). Both carry the data.values envelope.
func (s *Service) newRecordUpdateCmd(token string) *cobra.Command {
	var valuesFlag string
	var appendMode bool
	cmd := &cobra.Command{
		Use:   "update <object> <record_id> --values <json>",
		Short: "Update a record (default overwrite; --append to append multiselect)",
		Long: "Defaults to OVERWRITE: the values sent replace what is there, and a\n" +
			"multiselect attribute is reset to exactly what --values contains, dropping\n" +
			"anything not mentioned. --append switches to a patch that ADDS to\n" +
			"multiselect attributes instead. Attributes absent from --values are left\n" +
			"alone in either mode. `entry update` follows the same rule for list\n" +
			"entries.",
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
		payload := map[string]any{"data": map[string]any{"values": values}}
		method := http.MethodPut
		if appendMode {
			method = http.MethodPatch
		}
		body, err := s.call(cmd.Context(), token, method, recordPath(args[0], args[1]), payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newRecordUpsertCmd is `record upsert <object> --values <json> --match <attribute>`
// (PUT /v2/objects/{object}/records?matching_attribute=<attr>): assert by a
// unique matching attribute — create if absent, overwrite if present.
func (s *Service) newRecordUpsertCmd(token string) *cobra.Command {
	var valuesFlag, match string
	cmd := &cobra.Command{
		Use:   "upsert <object> --values <json> --match <attribute>",
		Short: "Assert (upsert) a record by a unique matching attribute",
		Long: "--match names the attribute to key on and must be one Attio treats as\n" +
			"unique — `attribute list` marks which are. A matching record is\n" +
			"OVERWRITTEN with the values sent, exactly as `record update` without\n" +
			"--append would; no match creates a new record. That makes this the\n" +
			"idempotent import path, and also means a wrong --match attribute\n" +
			"overwrites the wrong record without complaint.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&valuesFlag, "values", "", "JSON object of attribute slug/id → value (required)")
	cmd.Flags().StringVar(&match, "match", "", "slug/id of the unique attribute to match on (required)")
	_ = cmd.MarkFlagRequired("match")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseValuesFlag(valuesFlag)
		if err != nil {
			return err
		}
		payload := map[string]any{"data": map[string]any{"values": values}}
		q := url.Values{}
		q.Set("matching_attribute", match)
		path := "/v2/objects/" + url.PathEscape(args[0]) + "/records?" + q.Encode()
		body, err := s.call(cmd.Context(), token, http.MethodPut, path, payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// recordPath builds /v2/objects/{object}/records/{record_id}.
func recordPath(object, recordID string) string {
	return "/v2/objects/" + url.PathEscape(object) + "/records/" + url.PathEscape(recordID)
}
