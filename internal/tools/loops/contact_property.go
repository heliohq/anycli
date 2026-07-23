package loops

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactPropertyCmd groups custom-contact-property operations. Custom
// properties must exist before they can be set on a contact.
func (s *Service) newContactPropertyCmd(key string) *cobra.Command {
	cmd := newGroup("contact-property", "Custom contact properties (list, create)")
	cmd.AddCommand(
		s.newContactPropertyListCmd(key),
		s.newContactPropertyCreateCmd(key),
	)
	return cmd
}

func (s *Service) newContactPropertyListCmd(key string) *cobra.Command {
	var list string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List contact properties (GET /v1/contacts/properties)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("list", list)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/v1/contacts/properties", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&list, "list", "all", "which properties to list: all|custom")
	return cmd
}

func (s *Service) newContactPropertyCreateCmd(key string) *cobra.Command {
	var name, propType string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a custom contact property (POST /v1/contacts/properties)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name, "type": propType}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/v1/contacts/properties", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "property name in camelCase, e.g. planName (required)")
	cmd.Flags().StringVar(&propType, "type", "", "property type: string|number|boolean|date (required)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
