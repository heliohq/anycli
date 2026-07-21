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
		Args:  cobra.ExactArgs(2),
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.ExactArgs(2),
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
		Args:  cobra.ExactArgs(2),
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
		Args:  cobra.ExactArgs(3),
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
