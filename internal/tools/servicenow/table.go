package servicenow

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// queryOptions holds the shared read flags that map to sysparm_* query params.
type queryOptions struct {
	query        string
	fields       string
	limit        int
	offset       int
	displayValue string
}

// registerReadFlags attaches the read-shaping flags common to query/get.
func registerReadFlags(cmd *cobra.Command, o *queryOptions, withPaging bool) {
	cmd.Flags().StringVar(&o.fields, "fields", "", "comma-separated fields to return (sysparm_fields)")
	cmd.Flags().StringVar(&o.displayValue, "display-value", "", "resolve reference/choice fields: all|true|false (sysparm_display_value)")
	if withPaging {
		cmd.Flags().StringVar(&o.query, "query", "", "encoded query, e.g. active=true^priority=1 (sysparm_query)")
		cmd.Flags().IntVar(&o.limit, "limit", 0, "max records to return (sysparm_limit)")
		cmd.Flags().IntVar(&o.offset, "offset", 0, "records to skip (sysparm_offset)")
	}
}

// toValues turns the read options into sysparm_* query params, validating the
// display-value enum.
func (o *queryOptions) toValues() (url.Values, error) {
	v := url.Values{}
	if o.query != "" {
		v.Set("sysparm_query", o.query)
	}
	if o.fields != "" {
		v.Set("sysparm_fields", o.fields)
	}
	if o.limit > 0 {
		v.Set("sysparm_limit", strconv.Itoa(o.limit))
	}
	if o.offset > 0 {
		v.Set("sysparm_offset", strconv.Itoa(o.offset))
	}
	if o.displayValue != "" {
		switch o.displayValue {
		case "all", "true", "false":
			v.Set("sysparm_display_value", o.displayValue)
		default:
			return nil, &usageError{msg: fmt.Sprintf("--display-value must be all|true|false, got %q", o.displayValue)}
		}
	}
	return v, nil
}

func (s *Service) newTableQueryCmd(c *client) *cobra.Command {
	var o queryOptions
	cmd := &cobra.Command{
		Use:   "query <table>",
		Short: "List/query records in a table (GET)",
		Long: "--query takes an encoded query, not SQL: `^` for AND, `^OR` for OR,\n" +
			"operators `=`, `!=`, `LIKE`, `IN`, `>=`, `STARTSWITH`, and no spaces\n" +
			"anywhere around them. --fields is worth setting on every call — a bare\n" +
			"incident row carries around a hundred columns, most of them empty sys_ids.\n" +
			"--limit and --offset page the result, and omitting --limit can return an\n" +
			"enormous response. --display-value all turns reference and choice fields\n" +
			"into their human labels instead of sys_ids. Output is a bare JSON array\n" +
			"with the `{result}` envelope removed.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := o.toValues()
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodGet, args[0], "", v, nil)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	registerReadFlags(cmd, &o, true)
	return cmd
}

func (s *Service) newTableGetCmd(c *client) *cobra.Command {
	var o queryOptions
	cmd := &cobra.Command{
		Use:   "get <table> <sys_id>",
		Short: "Get one record by sys_id (GET)",
		Long: "The second argument must be a 32-hex sys_id; a human number such as\n" +
			"INC0010001 returns a 404 here, and translating one costs a `table query\n" +
			"<table> --query \"number=INC0010001\" --fields sys_id` first — or use\n" +
			"`incident get`, which does that lookup itself. --fields and\n" +
			"--display-value shape the response exactly as they do on a query. Output\n" +
			"is a bare JSON object.",
		Args:        cobra.ExactArgs(2),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := o.toValues()
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodGet, args[0], args[1], v, nil)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	registerReadFlags(cmd, &o, false)
	return cmd
}

func (s *Service) newTableCreateCmd(c *client) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "create <table>",
		Short: "Create a record (POST)",
		Long: "--data is the whole record as one JSON object, with reference fields given\n" +
			"as sys_ids rather than names — an assignment_group written as \"Network\" is\n" +
			"stored as literal text or rejected, not resolved. Creating a record runs\n" +
			"the table's business rules, workflows and notification policies, so this\n" +
			"can page an on-call human. The response is the created record including\n" +
			"the sys_id it was assigned.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := parseDataObject(data)
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodPost, args[0], "", nil, payload)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "record fields as a JSON object (required)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

func (s *Service) newTableUpdateCmd(c *client) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "update <table> <sys_id>",
		Short: "Update a record by sys_id (PATCH)",
		Long: "A PATCH: only the fields named in --data change and everything else\n" +
			"survives, so there is no need to send the whole record back. Reference\n" +
			"fields take sys_ids. Two journal fields behave differently from the rest —\n" +
			"`work_notes` appends an INTERNAL entry, while `comments` appends a\n" +
			"customer-visible one that typically notifies the caller, so the choice\n" +
			"between them is a choice about who gets emailed.",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := parseDataObject(data)
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodPatch, args[0], args[1], nil, payload)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "fields to change as a JSON object (required)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

func (s *Service) newTableDeleteCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <table> <sys_id>",
		Short: "Delete a record by sys_id (DELETE)",
		Long: "A real delete with no undo and no recycle bin reachable from this API;\n" +
			"whether related records cascade is decided by the table's own reference\n" +
			"rules. ServiceNow answers 204 with no body, so the command prints its own\n" +
			"`{\"deleted\":true,\"sys_id\":…}` receipt rather than provider JSON — that\n" +
			"receipt is not evidence the record existed beforehand. Retiring a record\n" +
			"rather than destroying it is usually a state change via `table update`.",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := c.callTable(cmd.Context(), http.MethodDelete, args[0], args[1], nil, nil); err != nil {
				return err
			}
			// Table API DELETE returns 204 no content; emit an explicit receipt.
			out, _ := json.Marshal(map[string]any{"deleted": true, "sys_id": args[1]})
			return s.emitJSON(out)
		},
	}
	return cmd
}

// parseDataObject decodes a --data JSON object, rejecting non-objects and empty
// input as usage errors.
func parseDataObject(data string) (map[string]any, error) {
	if strings.TrimSpace(data) == "" {
		return nil, &usageError{msg: "--data is required and must be a JSON object"}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--data is not valid JSON object: %v", err)}
	}
	return m, nil
}
