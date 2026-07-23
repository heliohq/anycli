package zohocrm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newRecordListCmd is `record list` — GET /crm/v8/{module}. The `fields` param
// is mandatory per the v8 contract, so --fields is a required flag and the
// error points the agent at `field list` first. --page and --page-token are
// mutually exclusive (the API rejects combining them). Output is the provider
// JSON verbatim.
func (s *Service) newRecordListCmd(token string) *cobra.Command {
	var module, fields, pageToken, sortBy, sortOrder string
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List records in a module (fields required)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name, e.g. Leads (required)")
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated field API names, max 50 (required; run `field list` to discover them)")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number (covers the first 2,000 records)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "next_page_token from a prior response (beyond record 2,000)")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "records per page (default/max 200)")
	cmd.Flags().StringVar(&sortBy, "sort-by", "", "sort field: id|Created_Time|Modified_Time")
	cmd.Flags().StringVar(&sortOrder, "sort-order", "", "sort direction: asc|desc")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		if strings.TrimSpace(fields) == "" {
			return &usageError{msg: "--fields is required for record list; run `zoho-crm field list --module " + strings.TrimSpace(module) + "` to discover field API names"}
		}
		if page > 0 && strings.TrimSpace(pageToken) != "" {
			return &usageError{msg: "--page and --page-token are mutually exclusive"}
		}
		if strings.TrimSpace(sortBy) != "" {
			switch sortBy {
			case "id", "Created_Time", "Modified_Time":
			default:
				return &usageError{msg: fmt.Sprintf("--sort-by must be one of id|Created_Time|Modified_Time, got %q", sortBy)}
			}
		}
		if strings.TrimSpace(sortOrder) != "" && sortOrder != "asc" && sortOrder != "desc" {
			return &usageError{msg: fmt.Sprintf("--sort-order must be asc or desc, got %q", sortOrder)}
		}
		q := url.Values{}
		setFieldsParam(q, fields)
		if page > 0 {
			q.Set("page", strconv.Itoa(page))
		}
		if strings.TrimSpace(pageToken) != "" {
			q.Set("page_token", pageToken)
		}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		if strings.TrimSpace(sortBy) != "" {
			q.Set("sort_by", sortBy)
		}
		if strings.TrimSpace(sortOrder) != "" {
			q.Set("sort_order", sortOrder)
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, modulePath(module)+"?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newRecordGetCmd is `record get` — GET /crm/v8/{module}/{id}. Optional
// --fields narrows the returned columns.
func (s *Service) newRecordGetCmd(token string) *cobra.Command {
	var module, id, fields string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a single record by id",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.Flags().StringVar(&id, "id", "", "record id (required)")
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated field API names to return")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		if strings.TrimSpace(id) == "" {
			return &usageError{msg: "--id is required"}
		}
		path := modulePath(module) + "/" + url.PathEscape(strings.TrimSpace(id))
		if strings.TrimSpace(fields) != "" {
			q := url.Values{}
			setFieldsParam(q, fields)
			path += "?" + q.Encode()
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newRecordCreateCmd is `record create` — POST /crm/v8/{module}. --data is a
// JSON object (one record) or array (bulk, ≤100); the service wraps it into
// {"data":[…]}. --no-triggers maps to "trigger":[] to suppress workflows.
// A 207 Multi-Status (mixed per-record outcome) is surfaced as success output.
func (s *Service) newRecordCreateCmd(token string) *cobra.Command {
	var module, data string
	var noTriggers bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create one or more records",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.Flags().StringVar(&data, "data", "", "JSON object or array of records (required)")
	cmd.Flags().BoolVar(&noTriggers, "no-triggers", false, "suppress workflow/approval/blueprint automations (trigger:[])")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		records, err := parseRecordData(data)
		if err != nil {
			return err
		}
		payload := writeEnvelope(records, noTriggers)
		body, err := s.call(cmd.Context(), token, http.MethodPost, modulePath(module), payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newRecordUpdateCmd is `record update` — PUT /crm/v8/{module}/{id}. --data is
// a single JSON object of fields to change; the id is taken from --id. Same
// --no-triggers semantics as create.
func (s *Service) newRecordUpdateCmd(token string) *cobra.Command {
	var module, id, data string
	var noTriggers bool
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a single record by id",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.Flags().StringVar(&id, "id", "", "record id (required)")
	cmd.Flags().StringVar(&data, "data", "", "JSON object of fields to update (required)")
	cmd.Flags().BoolVar(&noTriggers, "no-triggers", false, "suppress workflow/approval/blueprint automations (trigger:[])")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		if strings.TrimSpace(id) == "" {
			return &usageError{msg: "--id is required"}
		}
		records, err := parseSingleObject(data)
		if err != nil {
			return err
		}
		payload := writeEnvelope(records, noTriggers)
		path := modulePath(module) + "/" + url.PathEscape(strings.TrimSpace(id))
		body, err := s.call(cmd.Context(), token, http.MethodPut, path, payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newRecordDeleteCmd is `record delete`. With --id it deletes one record
// (DELETE /crm/v8/{module}/{id}); with --ids it bulk-deletes
// (DELETE /crm/v8/{module}?ids=id1,id2). Exactly one of --id / --ids is
// required.
func (s *Service) newRecordDeleteCmd(token string) *cobra.Command {
	var module, id, ids string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete one record (--id) or several (--ids)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.Flags().StringVar(&id, "id", "", "single record id")
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated record ids for bulk delete")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		hasID := strings.TrimSpace(id) != ""
		hasIDs := strings.TrimSpace(ids) != ""
		if hasID == hasIDs {
			return &usageError{msg: "exactly one of --id or --ids is required"}
		}
		var path string
		if hasID {
			path = modulePath(module) + "/" + url.PathEscape(strings.TrimSpace(id))
		} else {
			q := url.Values{}
			q.Set("ids", ids)
			path = modulePath(module) + "?" + q.Encode()
		}
		body, err := s.call(cmd.Context(), token, http.MethodDelete, path, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newRecordSearchCmd is `record search` — GET /crm/v8/{module}/search. Exactly
// one of --criteria / --email / --phone / --word is required (the v8 API
// silently prioritizes when several are given; the CLI turns that into a hard
// error). Requires the ZohoSearch.securesearch.READ scope. An empty result is
// a 204 that emits nothing and exits 0.
func (s *Service) newRecordSearchCmd(token string) *cobra.Command {
	var module, criteria, email, phone, word, fields string
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search records by criteria, email, phone, or word",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.Flags().StringVar(&criteria, "criteria", "", "criteria expression, e.g. (Last_Name:equals:Doe)")
	cmd.Flags().StringVar(&email, "email", "", "search by email value")
	cmd.Flags().StringVar(&phone, "phone", "", "search by phone value")
	cmd.Flags().StringVar(&word, "word", "", "free-text word search")
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated field API names to return")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "records per page (default/max 200)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		selectors := map[string]string{"criteria": criteria, "email": email, "phone": phone, "word": word}
		set := 0
		for _, v := range selectors {
			if strings.TrimSpace(v) != "" {
				set++
			}
		}
		if set != 1 {
			return &usageError{msg: "exactly one of --criteria, --email, --phone, or --word is required"}
		}
		q := url.Values{}
		for k, v := range selectors {
			if strings.TrimSpace(v) != "" {
				q.Set(k, v)
			}
		}
		setFieldsParam(q, fields)
		if page > 0 {
			q.Set("page", strconv.Itoa(page))
		}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, modulePath(module)+"/search?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// modulePath renders /{module} with the module API name path-escaped.
func modulePath(module string) string {
	return "/" + url.PathEscape(strings.TrimSpace(module))
}

// writeEnvelope builds the {"data":[…]} request body, adding "trigger":[] when
// automations are suppressed.
func writeEnvelope(records []json.RawMessage, noTriggers bool) map[string]any {
	payload := map[string]any{"data": records}
	if noTriggers {
		payload["trigger"] = []any{}
	}
	return payload
}
