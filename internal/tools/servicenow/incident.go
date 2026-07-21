package servicenow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// incidentTable is the ServiceNow incident table name.
const incidentTable = "incident"

// stateResolved is the ServiceNow incident "Resolved" state value.
const stateResolved = "6"

// sysIDRe matches a 32-hex ServiceNow sys_id. A value that is not a sys_id is
// treated as a human incident number (INC0010001) and resolved via lookup.
var sysIDRe = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

func (s *Service) newIncidentListCmd(c *client) *cobra.Command {
	var o queryOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List incidents (query sugar over the incident table)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v, err := o.toValues()
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodGet, incidentTable, "", v, nil)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	registerReadFlags(cmd, &o, true)
	return cmd
}

func (s *Service) newIncidentGetCmd(c *client) *cobra.Command {
	var o queryOptions
	cmd := &cobra.Command{
		Use:   "get <number|sys_id>",
		Short: "Get one incident by INC number or sys_id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sysID, err := c.resolveIncidentSysID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := o.toValues()
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodGet, incidentTable, sysID, v, nil)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	registerReadFlags(cmd, &o, false)
	return cmd
}

func (s *Service) newIncidentCreateCmd(c *client) *cobra.Command {
	var shortDescription, data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an incident",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{}
			if strings.TrimSpace(data) != "" {
				parsed, err := parseDataObject(data)
				if err != nil {
					return err
				}
				payload = parsed
			}
			if cmd.Flags().Changed("short-description") {
				payload["short_description"] = shortDescription
			}
			if len(payload) == 0 {
				return &usageError{msg: "incident create needs --short-description or --data"}
			}
			body, err := c.callTable(cmd.Context(), http.MethodPost, incidentTable, "", nil, payload)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	cmd.Flags().StringVar(&shortDescription, "short-description", "", "incident short description")
	cmd.Flags().StringVar(&data, "data", "", "additional incident fields as a JSON object")
	return cmd
}

func (s *Service) newIncidentUpdateCmd(c *client) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "update <number|sys_id>",
		Short: "Update an incident by INC number or sys_id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := parseDataObject(data)
			if err != nil {
				return err
			}
			sysID, err := c.resolveIncidentSysID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodPatch, incidentTable, sysID, nil, payload)
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

func (s *Service) newIncidentResolveCmd(c *client) *cobra.Command {
	var closeNotes, code string
	cmd := &cobra.Command{
		Use:   "resolve <number|sys_id>",
		Short: "Resolve an incident (sets state=Resolved with close notes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(closeNotes) == "" {
				return &usageError{msg: "--close-notes is required to resolve an incident"}
			}
			payload := map[string]any{
				"state":       stateResolved,
				"close_notes": closeNotes,
			}
			if strings.TrimSpace(code) != "" {
				payload["close_code"] = code
			}
			sysID, err := c.resolveIncidentSysID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			body, err := c.callTable(cmd.Context(), http.MethodPatch, incidentTable, sysID, nil, payload)
			if err != nil {
				return err
			}
			return s.emitResult(body)
		},
	}
	cmd.Flags().StringVar(&closeNotes, "close-notes", "", "resolution notes (required)")
	cmd.Flags().StringVar(&code, "code", "", "close/resolution code (e.g. 'Solved (Permanently)')")
	return cmd
}

// resolveIncidentSysID returns ref unchanged when it is already a 32-hex sys_id;
// otherwise it treats ref as a human incident number (INC0010001) and looks up
// the matching record's sys_id via sysparm_query=number=<ref>&sysparm_limit=1.
// A number with no match is a usage error (the caller named a nonexistent
// incident), not an API failure.
func (c *client) resolveIncidentSysID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", &usageError{msg: "empty incident number/sys_id"}
	}
	if sysIDRe.MatchString(ref) {
		return ref, nil
	}
	v := url.Values{}
	v.Set("sysparm_query", "number="+ref)
	v.Set("sysparm_limit", "1")
	v.Set("sysparm_fields", "sys_id")
	body, err := c.callTable(ctx, http.MethodGet, incidentTable, "", v, nil)
	if err != nil {
		return "", err
	}
	result, err := unwrapResult(body)
	if err != nil {
		return "", err
	}
	var rows []struct {
		SysID string `json:"sys_id"`
	}
	if err := json.Unmarshal(result, &rows); err != nil {
		return "", &apiError{msg: fmt.Sprintf("servicenow: decode incident lookup: %v", err), err: err}
	}
	if len(rows) == 0 || rows[0].SysID == "" {
		return "", &usageError{msg: fmt.Sprintf("no incident found with number %q", ref)}
	}
	return rows[0].SysID, nil
}
