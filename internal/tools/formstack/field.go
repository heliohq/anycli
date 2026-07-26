package formstack

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newFieldCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "field", Short: "Form fields (get, create)"}
	cmd.AddCommand(
		s.newFieldGetCmd(token),
		s.newFieldCreateCmd(token),
	)
	return cmd
}

func (s *Service) newFieldGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <field-id>",
		Short: "Get a field (GET /field/{id}.json)",
		Long: "Takes a FIELD id from `form fields`, not the id of the form it belongs\n" +
			"to. Returns that one field's full definition — `type`, `label`,\n" +
			"`required`, `hidden`, the `options` list for choice fields and any\n" +
			"conditional logic on it. Reading a whole form's structure is one call\n" +
			"with `form fields`; this is for drilling into a single question.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/field/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newFieldCreateCmd(token string) *cobra.Command {
	var fieldType, label string
	var options []string
	var required, hidden bool
	cmd := &cobra.Command{
		Use:   "create <form-id>",
		Short: "Create a field on a form (POST /form/{id}/field.json)",
		Long: "`--type` and `--label` are both required and the type is fixed at\n" +
			"creation. Common types are `text`, `textarea`, `email`, `number`,\n" +
			"`select`, `radio`, `checkbox`, `datetime`, `phone` and `name`.\n" +
			"`--options` only means anything for the choice types and is split on\n" +
			"commas, so an option whose own text contains a comma cannot be expressed\n" +
			"here. The field is appended to the form; ordering, layout and conditional\n" +
			"logic stay in the Formstack builder. Keep the returned `id` — submitted\n" +
			"answers are keyed by it, not by the label.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"field_type": fieldType,
				"label":      label,
			}
			if required {
				body["required"] = true
			}
			if hidden {
				body["hidden"] = true
			}
			if len(options) > 0 {
				body["options"] = options
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/form/"+url.PathEscape(args[0])+"/field.json", nil, body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&fieldType, "type", "", "field type (text, email, select, ...)")
	cmd.Flags().StringVar(&label, "label", "", "field label")
	cmd.Flags().StringSliceVar(&options, "options", nil, "options for select/radio/checkbox (comma-separated)")
	cmd.Flags().BoolVar(&required, "required", false, "mark the field required")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "mark the field hidden")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}
