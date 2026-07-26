package salesforce

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// readData resolves a --data flag value into a raw JSON body. A leading "@"
// reads a file ("@-" reads stdin); anything else is a literal JSON string. The
// payload is validated as JSON before it is sent so a malformed body is a usage
// error (exit 2), not a wasted round-trip.
func readData(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, &usageError{msg: "--data is required (literal JSON, @file, or @- for stdin)"}
	}
	var raw []byte
	switch {
	case value == "@-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --data from stdin: %v", err)}
		}
		raw = b
	case strings.HasPrefix(value, "@"):
		b, err := os.ReadFile(value[1:])
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --data file: %v", err)}
		}
		raw = b
	default:
		raw = []byte(value)
	}
	if !json.Valid(raw) {
		return nil, &usageError{msg: "--data is not valid JSON"}
	}
	return raw, nil
}

func (s *Service) newRecordGetCmd(c *client) *cobra.Command {
	var fields []string
	cmd := &cobra.Command{
		Use:   "get <sobject> <id>",
		Short: "Retrieve one record by id",
		Long: "Takes the sObject API name and the record id positionally, in that order —\n" +
			"`record get Account 001…`, never the label a user sees. Without `--fields`\n" +
			"the org returns EVERY field on the record, which on a customised Account is\n" +
			"hundreds of them, so name the fields wanted. This is an id lookup only:\n" +
			"finding a record by name or email is `query` or `search`.",
		Args:        cobra.ExactArgs(2),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := dataPath(apiVersion(cmd), "/sobjects/"+url.PathEscape(args[0])+"/"+url.PathEscape(args[1]))
			if len(fields) > 0 {
				path += "?fields=" + url.QueryEscape(strings.Join(fields, ","))
			}
			body, _, err := c.get(cmd.Context(), path)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "only return these fields")
	return cmd
}

func (s *Service) newRecordCreateCmd(c *client) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "create <sobject>",
		Short: "Create a record",
		Long: "`--data` carries the field JSON and accepts a literal string, `@file`, or\n" +
			"`@-` for stdin. It is parsed before anything is sent, so malformed JSON\n" +
			"fails as a usage error without touching the org. Returns `id`, `success`\n" +
			"and `errors`, with the new record's 18-character id. Which fields are\n" +
			"mandatory is org configuration, not a Salesforce constant — read\n" +
			"`sobject describe <Object>` rather than assuming, and expect validation\n" +
			"rules and triggers the payload cannot see.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readData(data)
			if err != nil {
				return err
			}
			path := dataPath(apiVersion(cmd), "/sobjects/"+url.PathEscape(args[0]))
			body, _, callErr := c.call(cmd.Context(), http.MethodPost, path, payload)
			if callErr != nil {
				return callErr
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "record fields as JSON (literal, @file, or @-)")
	return cmd
}

func (s *Service) newRecordUpdateCmd(c *client) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "update <sobject> <id>",
		Short: "Update a record (PATCH; 204 No Content on success)",
		Long: "PATCH semantics: only the fields present in `--data` are touched, so a\n" +
			"one-field payload is the normal case and there is no read-modify-write.\n" +
			"Clearing a field therefore means sending it explicitly as null — omitting\n" +
			"it leaves the old value. Salesforce answers with an empty 204, so the tool\n" +
			"synthesizes `{\"success\": true, \"id\": …}` and stdout still carries a result.\n" +
			"`--data` accepts a literal string, `@file`, or `@-` for stdin.",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readData(data)
			if err != nil {
				return err
			}
			path := dataPath(apiVersion(cmd), "/sobjects/"+url.PathEscape(args[0])+"/"+url.PathEscape(args[1]))
			body, status, callErr := c.call(cmd.Context(), http.MethodPatch, path, payload)
			if callErr != nil {
				return callErr
			}
			return s.emitWriteResult(body, status, args[1])
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "changed fields as JSON (literal, @file, or @-)")
	return cmd
}

func (s *Service) newRecordDeleteCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <sobject> <id>",
		Short: "Delete a record (204 No Content on success)",
		Long: "The record goes to the org's Recycle Bin rather than being erased, so a\n" +
			"person can restore it in Salesforce and `query --all` still sees it. The\n" +
			"delete CASCADES: removing an Account takes its Contacts, Opportunities and\n" +
			"Cases with it, which is far more than the one id names. Salesforce answers\n" +
			"with an empty 204 and the tool synthesizes `{\"success\": true, \"id\": …}`.",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := dataPath(apiVersion(cmd), "/sobjects/"+url.PathEscape(args[0])+"/"+url.PathEscape(args[1]))
			body, status, err := c.call(cmd.Context(), http.MethodDelete, path, nil)
			if err != nil {
				return err
			}
			return s.emitWriteResult(body, status, args[1])
		},
	}
}

func (s *Service) newRecordUpsertCmd(c *client) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "upsert <sobject> <ext-id-field> <value>",
		Short: "Upsert a record by external id field",
		Long: "Three positional arguments: the sObject, the API name of a field marked\n" +
			"EXTERNAL ID in the org, and the value to match on. It is not a record id\n" +
			"and not any arbitrary field — Salesforce rejects a field that lacks the\n" +
			"External Id attribute. No match creates, exactly one match updates, and\n" +
			"several matches is an error rather than a pick, which is what makes this\n" +
			"the idempotent way to sync records in from an outside system. A create\n" +
			"answers 201 with `created: true`; a matched update can answer an empty 204,\n" +
			"which the tool renders as a bare `{\"success\": true}` carrying no id.",
		Args:        cobra.ExactArgs(3),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readData(data)
			if err != nil {
				return err
			}
			path := dataPath(apiVersion(cmd), "/sobjects/"+url.PathEscape(args[0])+"/"+url.PathEscape(args[1])+"/"+url.PathEscape(args[2]))
			body, status, callErr := c.call(cmd.Context(), http.MethodPatch, path, payload)
			if callErr != nil {
				return callErr
			}
			// Upsert returns 200/201 with a body ({id,success,created}); a 204
			// (matched, no fields changed) has no body.
			return s.emitWriteResult(body, status, "")
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "record fields as JSON (literal, @file, or @-)")
	return cmd
}

// emitWriteResult emits the provider body verbatim when present, and synthesizes
// a {"success":true[,"id":…]} envelope for the 204 No Content writes (update /
// delete / no-op upsert) that Salesforce answers with an empty body.
func (s *Service) emitWriteResult(body []byte, status int, id string) error {
	if len(strings.TrimSpace(string(body))) > 0 {
		return s.emit(body)
	}
	result := map[string]any{"success": true}
	if id != "" {
		result["id"] = id
	}
	out, err := json.Marshal(result)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("salesforce: encode write result: %v", err), err: err}
	}
	return s.emit(out)
}
