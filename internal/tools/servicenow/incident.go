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
		Long: "Identical to `table query incident` — same encoded-query syntax, same\n" +
			"--fields, --limit, --offset and --display-value, one less argument to\n" +
			"type. Incidents are heavy records, so --fields\n" +
			"number,short_description,state,assigned_to keeps the response readable\n" +
			"where an unfiltered call does not.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes either form: a 32-hex value is used directly as a sys_id, and\n" +
			"anything else is treated as the human INC number and translated by an\n" +
			"extra lookup request. That translation is why this is the command to use\n" +
			"when a person named an incident — the generic `table get` accepts only the\n" +
			"sys_id. A number with no match fails as a usage error rather than an empty\n" +
			"result.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "At least one of --short-description or --data is required. Everything\n" +
			"beyond the summary line — urgency, impact, caller_id, assignment_group,\n" +
			"category — goes in --data as a JSON object, with reference fields written\n" +
			"as sys_ids. Creating an incident triggers the instance's assignment rules\n" +
			"and notifications, so it can page whoever is on call for the resulting\n" +
			"group.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Accepts an INC number or a sys_id and PATCHes only the fields --data\n" +
			"names. Reassigning means writing assigned_to or assignment_group as\n" +
			"sys_ids, not names. `work_notes` adds an internal note while `comments`\n" +
			"adds a customer-visible one that usually emails the caller — pick\n" +
			"deliberately. Marking an incident resolved belongs in `incident resolve`,\n" +
			"which sets the state and close fields together.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "Sets state to 6, Resolved, and --close-notes is required because resolving\n" +
			"without a stated resolution is rejected. --code writes close_code, whose\n" +
			"valid values are the instance's own choice list (\"Solved (Permanently)\"\n" +
			"and similar), so a value that instance does not define will not stick.\n" +
			"Resolved is not Closed: the record stays open to the caller for rebuttal,\n" +
			"and moving it further is a separate `incident update`. Resolving notifies\n" +
			"the caller on most instances.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
