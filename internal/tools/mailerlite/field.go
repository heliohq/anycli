package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newFieldCmd builds the `mailerlite field` command tree — custom fields must
// be discoverable before a subscriber write can set them.
func (s *Service) newFieldCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "field", Short: "Custom fields (list, create, update, delete)"}
	cmd.AddCommand(
		s.newFieldListCmd(token),
		s.newFieldCreateCmd(token),
		s.newFieldUpdateCmd(token),
		s.newFieldDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newFieldListCmd(token string) *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List custom fields (GET /fields)",
		Long: "The prerequisite for writing custom data: `subscriber create --fields` and\n" +
			"`subscriber update --fields` take a JSON object whose keys must match\n" +
			"fields that already exist, and an unrecognised key is not created\n" +
			"implicitly. Page-numbered with --page.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/fields", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}

func (s *Service) newFieldCreateCmd(token string) *cobra.Command {
	var name, fieldType string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a custom field (POST /fields)",
		Long: "--type is text, number or date and is fixed at creation — `field update`\n" +
			"renames a field but cannot change its type, so a wrong type has to be\n" +
			"deleted and rebuilt, losing every stored value. Pick the type from how the\n" +
			"data will be filtered later, not from how it happens to be formatted now.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/fields", nil, map[string]any{"name": name, "type": fieldType})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "field name (required)")
	cmd.Flags().StringVar(&fieldType, "type", "", "field type: text|number|date (required)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func (s *Service) newFieldUpdateCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Rename a custom field (PUT /fields/{id})",
		Long: "Renaming only — the field's type and every stored value survive. Note that\n" +
			"subscriber writes address fields by key, so a rename can break an\n" +
			"integration that still sends the old name.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/fields/"+url.PathEscape(args[0]), nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new field name (required)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newFieldDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a custom field (DELETE /fields/{id})",
		Long: "Destroys the field along with its stored value for EVERY subscriber, and\n" +
			"recreating a field with the same name does not bring the data back.\n" +
			"Campaign content that personalises on the field renders empty afterwards.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/fields/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
