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
		Args:  cobra.ExactArgs(1),
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
		Long: "Create a field on a form. Common --type values: text, textarea, " +
			"email, number, select, radio, checkbox, datetime, phone, name. " +
			"Advanced layout/logic stays in the Formstack builder.",
		Args: cobra.ExactArgs(1),
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
