package salesforce

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// Full describe / global-list payloads are hundreds of KB — pathological for a
// context window. list and describe emit a trimmed projection by default; --raw
// returns the untrimmed provider body.

type globalSObjects struct {
	SObjects []map[string]any `json:"sobjects"`
}

// trimmedSObject is the projection `sobject list` emits per object.
type trimmedSObject struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Custom    bool   `json:"custom"`
	Queryable bool   `json:"queryable"`
}

func (s *Service) newSObjectListCmd(c *client) *cobra.Command {
	var customOnly, standardOnly, raw bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the org's sObjects (trimmed; --raw for full)",
		Long: "Emits a projection of `name`, `label`, `custom` and `queryable` per object,\n" +
			"because the org's full global describe runs to hundreds of KB; `--raw`\n" +
			"returns that untrimmed body. `--custom-only` and `--standard-only` are\n" +
			"mutually exclusive and passing both is a usage error. `name` is the API\n" +
			"name every other command takes and it is NOT the label a user sees — what\n" +
			"the team calls \"Deals\" may be `Opportunity` or a custom `Deal__c`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if customOnly && standardOnly {
				return &usageError{msg: "--custom-only and --standard-only are mutually exclusive"}
			}
			body, _, err := c.get(cmd.Context(), dataPath(apiVersion(cmd), "/sobjects"))
			if err != nil {
				return err
			}
			if raw {
				return s.emit(body)
			}
			var global globalSObjects
			if err := json.Unmarshal(body, &global); err != nil {
				return &apiError{msg: fmt.Sprintf("salesforce: decode sobjects: %v", err), err: err}
			}
			trimmed := make([]trimmedSObject, 0, len(global.SObjects))
			for _, o := range global.SObjects {
				custom, _ := o["custom"].(bool)
				if customOnly && !custom {
					continue
				}
				if standardOnly && custom {
					continue
				}
				queryable, _ := o["queryable"].(bool)
				name, _ := o["name"].(string)
				label, _ := o["label"].(string)
				trimmed = append(trimmed, trimmedSObject{Name: name, Label: label, Custom: custom, Queryable: queryable})
			}
			out, err := json.Marshal(map[string]any{"sobjects": trimmed})
			if err != nil {
				return &apiError{msg: fmt.Sprintf("salesforce: encode sobjects: %v", err), err: err}
			}
			return s.emit(out)
		},
	}
	cmd.Flags().BoolVar(&customOnly, "custom-only", false, "only custom objects")
	cmd.Flags().BoolVar(&standardOnly, "standard-only", false, "only standard objects")
	cmd.Flags().BoolVar(&raw, "raw", false, "return the full untrimmed describe body")
	return cmd
}

type describeResult struct {
	Name   string           `json:"name"`
	Label  string           `json:"label"`
	Fields []map[string]any `json:"fields"`
}

// trimmedField is the projection `sobject describe` emits per field so the
// model can write valid SOQL and payloads without the full describe blob.
type trimmedField struct {
	Name           string `json:"name"`
	Label          string `json:"label"`
	Type           string `json:"type"`
	Required       bool   `json:"required"`
	Updateable     bool   `json:"updateable"`
	PicklistValues []any  `json:"picklistValues,omitempty"`
}

func (s *Service) newSObjectDescribeCmd(c *client) *cobra.Command {
	var fieldNamesOnly, raw bool
	cmd := &cobra.Command{
		Use:   "describe <sobject>",
		Short: "Describe an sObject's fields (trimmed; --raw for full)",
		Long: "Emits one entry per field — `name`, `label`, `type`, `required`,\n" +
			"`updateable`, and picklist values where the field has them — trimmed from a\n" +
			"describe body that is hundreds of KB on a mature object. `--raw` returns\n" +
			"the whole thing and `--field-names-only` cuts it to bare API names.\n" +
			"`required` is derived as \"not nillable and not defaulted on create\", which\n" +
			"is exactly what a `record create` payload must satisfy. Picklist fields\n" +
			"reject values outside their list, so read the values here before writing\n" +
			"one.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := dataPath(apiVersion(cmd), "/sobjects/"+url.PathEscape(args[0])+"/describe")
			body, _, err := c.get(cmd.Context(), path)
			if err != nil {
				return err
			}
			if raw {
				return s.emit(body)
			}
			var desc describeResult
			if err := json.Unmarshal(body, &desc); err != nil {
				return &apiError{msg: fmt.Sprintf("salesforce: decode describe: %v", err), err: err}
			}
			if fieldNamesOnly {
				names := make([]string, 0, len(desc.Fields))
				for _, f := range desc.Fields {
					if name, _ := f["name"].(string); name != "" {
						names = append(names, name)
					}
				}
				out, mErr := json.Marshal(map[string]any{"name": desc.Name, "label": desc.Label, "fields": names})
				if mErr != nil {
					return &apiError{msg: fmt.Sprintf("salesforce: encode describe: %v", mErr), err: mErr}
				}
				return s.emit(out)
			}
			fields := make([]trimmedField, 0, len(desc.Fields))
			for _, f := range desc.Fields {
				fields = append(fields, trimField(f))
			}
			out, err := json.Marshal(map[string]any{"name": desc.Name, "label": desc.Label, "fields": fields})
			if err != nil {
				return &apiError{msg: fmt.Sprintf("salesforce: encode describe: %v", err), err: err}
			}
			return s.emit(out)
		},
	}
	cmd.Flags().BoolVar(&fieldNamesOnly, "field-names-only", false, "only field API names")
	cmd.Flags().BoolVar(&raw, "raw", false, "return the full untrimmed describe body")
	return cmd
}

// trimField projects one describe field. required approximates Salesforce's
// create-time requiredness: not nillable and not defaulted on create.
func trimField(f map[string]any) trimmedField {
	name, _ := f["name"].(string)
	label, _ := f["label"].(string)
	typ, _ := f["type"].(string)
	updateable, _ := f["updateable"].(bool)
	nillable, _ := f["nillable"].(bool)
	defaulted, _ := f["defaultedOnCreate"].(bool)
	tf := trimmedField{
		Name:       name,
		Label:      label,
		Type:       typ,
		Required:   !nillable && !defaulted,
		Updateable: updateable,
	}
	if pv, ok := f["picklistValues"].([]any); ok && len(pv) > 0 {
		tf.PicklistValues = pv
	}
	return tf
}
