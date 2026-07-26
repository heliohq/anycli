package netsuite

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// pageParams appends limit/offset query params when set (>0 / >=0-and-changed).
// SuiteQL and record list both paginate via limit/offset; the params fold into
// the OAuth signature base string (sign.go), so they must be built here, not
// added as headers.
func pageParams(limit, offset int, offsetChanged bool) url.Values {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 || (offsetChanged && offset >= 0) {
		q.Set("offset", strconv.Itoa(offset))
	}
	return q
}

func withQuery(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// newQueryCmd runs a SuiteQL query — the workhorse for joined/aggregate reads.
// SuiteQL requires the header `Prefer: transient`; omitting it returns
// INVALID_HEADER, so the service always sets it.
func (s *Service) newQueryCmd(creds tbaCreds) *cobra.Command {
	var q string
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "query",
		Annotations: map[string]string{"anycli.side_effect": "false"},
		Short:       "Run a SuiteQL query (joined/aggregate reads across records)",
		Long: "--q takes a SuiteQL SELECT; the endpoint accepts no other verb, so\n" +
			"changing data always means a `record` subcommand. SuiteQL addresses\n" +
			"tables and columns by their DATABASE names, which are generally\n" +
			"lowercase (companyname, tranid) and differ from the camelCase field\n" +
			"names `record get` returns — a column copied from a record response\n" +
			"will often not resolve.\n" +
			"\n" +
			"The response carries `items` plus `hasMore`, `count`, `totalResults`\n" +
			"and `links`, where a rel:next link means another page exists. --limit\n" +
			"and --offset are applied by NetSuite around the statement, so a\n" +
			"statement's own FETCH FIRST ... ROWS ONLY and --limit are two\n" +
			"independent caps and the smaller one wins.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(q) == "" {
				return &usageError{msg: "netsuite query: --q (SuiteQL statement) is required"}
			}
			body, err := json.Marshal(map[string]string{"q": q})
			if err != nil {
				return &usageError{msg: fmt.Sprintf("netsuite query: encode statement: %v", err)}
			}
			path := withQuery("/query/v1/suiteql", pageParams(limit, offset, cmd.Flags().Changed("offset")))
			resp, err := s.call(cmd.Context(), creds, "POST", path, body, map[string]string{"Prefer": "transient"})
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "SuiteQL statement, e.g. \"SELECT id, companyname FROM customer\"")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows per page")
	cmd.Flags().IntVar(&offset, "offset", 0, "row offset for pagination")
	return cmd
}

// newMetadataCmd surfaces the record metadata catalog so the AI can discover
// record types/fields before reading or writing. With --type it fetches one
// record type's schema.
func (s *Service) newMetadataCmd(creds tbaCreds) *cobra.Command {
	var recordType string
	cmd := &cobra.Command{
		Use:         "metadata",
		Annotations: map[string]string{"anycli.side_effect": "false"},
		Short:       "Discover record types and field schemas (metadata catalog)",
		Long: "Without --type this returns the entire record-type catalog, which is\n" +
			"large; --type narrows it to one type's schema and is the usual call.\n" +
			"This is where an exact type name and its field names come from, so it\n" +
			"is the fix for a rejected --type or a create body NetSuite says carries\n" +
			"unknown fields. The payload is JSON Schema, not a record — field names\n" +
			"and requiredness, no data.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/record/v1/metadata-catalog"
			if t := strings.TrimSpace(recordType); t != "" {
				path += "/" + url.PathEscape(t)
			}
			// The catalog is served as application/schema+json; NetSuite honors
			// the Accept override for the catalog endpoint.
			resp, err := s.call(cmd.Context(), creds, "GET", path, nil, map[string]string{"Accept": "application/schema+json"})
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&recordType, "type", "", "record type to describe (omit for the full catalog)")
	return cmd
}

// requireType validates the shared --type flag on record subcommands.
func requireType(recordType string) (string, error) {
	t := strings.TrimSpace(recordType)
	if t == "" {
		return "", &usageError{msg: "netsuite record: --type (record type, e.g. customer) is required"}
	}
	return t, nil
}

// requireID validates the shared --id flag on record get/update/delete.
func requireID(id string) (string, error) {
	v := strings.TrimSpace(id)
	if v == "" {
		return "", &usageError{msg: "netsuite record: --id (internal id) is required"}
	}
	return v, nil
}

func (s *Service) newRecordGetCmd(creds tbaCreds) *cobra.Command {
	var recordType, id string
	cmd := &cobra.Command{
		Use:         "get",
		Annotations: map[string]string{"anycli.side_effect": "false"},
		Short:       "Get one record by internal id",
		Long: "--type and --id are both required, and --id is NetSuite's INTERNAL id —\n" +
			"not a tranId, document number or customer-facing entity id, which have\n" +
			"to be resolved through `query` first. The response holds the record's\n" +
			"own fields; sublists such as line items and addresses arrive as links\n" +
			"rather than expanded data, so a line-level question is a SuiteQL\n" +
			"question.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, err := requireType(recordType)
			if err != nil {
				return err
			}
			rid, err := requireID(id)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), creds, "GET", "/record/v1/"+url.PathEscape(t)+"/"+url.PathEscape(rid), nil, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&recordType, "type", "", "record type, e.g. customer")
	cmd.Flags().StringVar(&id, "id", "", "record internal id")
	return cmd
}

func (s *Service) newRecordListCmd(creds tbaCreds) *cobra.Command {
	var recordType string
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "list",
		Annotations: map[string]string{"anycli.side_effect": "false"},
		Short:       "List record ids of a type (paginated)",
		Long: "Returns ids and links, NOT record contents, so reading fields costs one\n" +
			"`record get` per row — which is why anything about several records\n" +
			"belongs in `query` instead. There is no filter flag: narrowing by any\n" +
			"field means SuiteQL. --limit and --offset page it, and --limit 0 leaves\n" +
			"NetSuite's own default page size in place.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, err := requireType(recordType)
			if err != nil {
				return err
			}
			path := withQuery("/record/v1/"+url.PathEscape(t), pageParams(limit, offset, cmd.Flags().Changed("offset")))
			resp, err := s.call(cmd.Context(), creds, "GET", path, nil, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&recordType, "type", "", "record type, e.g. customer")
	cmd.Flags().IntVar(&limit, "limit", 0, "max records per page")
	cmd.Flags().IntVar(&offset, "offset", 0, "record offset for pagination")
	return cmd
}

// parseBody validates the --body JSON on record create/update. Invalid JSON is
// a fail-fast usage error.
func parseBody(raw string) ([]byte, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, &usageError{msg: "netsuite record: --body (record JSON) is required"}
	}
	var probe json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("netsuite record: --body is not valid JSON: %v", err)}
	}
	return []byte(raw), nil
}

func (s *Service) newRecordCreateCmd(creds tbaCreds) *cobra.Command {
	var recordType, body string
	cmd := &cobra.Command{
		Use:         "create",
		Annotations: map[string]string{"anycli.side_effect": "true"},
		Short:       "Create a record",
		Long: "--body is the record's fields as JSON and is checked here only for\n" +
			"syntax; which fields are required varies by type and by the account's\n" +
			"configuration (a customer on a OneWorld account needs `subsidiary`), and\n" +
			"`metadata --type` is what lists them. NetSuite answers with an empty\n" +
			"body and the new record's URL in a Location header, so the printed id\n" +
			"is reconstructed from that header rather than read from a returned\n" +
			"record. The record is live immediately; there is no draft state.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, err := requireType(recordType)
			if err != nil {
				return err
			}
			payload, err := parseBody(body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), creds, "POST", "/record/v1/"+url.PathEscape(t), payload, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&recordType, "type", "", "record type, e.g. customer")
	cmd.Flags().StringVar(&body, "body", "", "record fields as JSON")
	return cmd
}

func (s *Service) newRecordUpdateCmd(creds tbaCreds) *cobra.Command {
	var recordType, id, body string
	cmd := &cobra.Command{
		Use:         "update",
		Annotations: map[string]string{"anycli.side_effect": "true"},
		Short:       "Update a record (PATCH by internal id)",
		Long: "A PATCH: fields absent from --body are left alone, so send only what is\n" +
			"changing. No version or If-Match check is sent with the request, which\n" +
			"means a concurrent edit by a person or another integration is\n" +
			"overwritten silently — re-read with `record get` immediately before\n" +
			"writing when that matters.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, err := requireType(recordType)
			if err != nil {
				return err
			}
			rid, err := requireID(id)
			if err != nil {
				return err
			}
			payload, err := parseBody(body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), creds, "PATCH", "/record/v1/"+url.PathEscape(t)+"/"+url.PathEscape(rid), payload, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&recordType, "type", "", "record type, e.g. customer")
	cmd.Flags().StringVar(&id, "id", "", "record internal id")
	cmd.Flags().StringVar(&body, "body", "", "changed fields as JSON")
	return cmd
}

func (s *Service) newRecordDeleteCmd(creds tbaCreds) *cobra.Command {
	var recordType, id string
	cmd := &cobra.Command{
		Use:         "delete",
		Annotations: map[string]string{"anycli.side_effect": "true"},
		Short:       "Delete a record by internal id",
		Long: "Immediate and permanent — NetSuite has no recycle bin behind this call.\n" +
			"It refuses rather than cascading when other records reference the\n" +
			"target, so the common failure is a dependency error and not a\n" +
			"permissions one. Many posted transactions cannot be deleted at all once\n" +
			"their accounting period is closed; those are voided inside NetSuite\n" +
			"instead, which this tool cannot do.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, err := requireType(recordType)
			if err != nil {
				return err
			}
			rid, err := requireID(id)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), creds, "DELETE", "/record/v1/"+url.PathEscape(t)+"/"+url.PathEscape(rid), nil, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&recordType, "type", "", "record type, e.g. customer")
	cmd.Flags().StringVar(&id, "id", "", "record internal id")
	return cmd
}
