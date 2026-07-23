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
		Use:         "list",
		Short:       "List custom fields (GET /fields)",
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
		Use:         "create",
		Short:       "Create a custom field (POST /fields)",
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
		Use:         "update <id>",
		Short:       "Rename a custom field (PUT /fields/{id})",
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
		Use:         "delete <id>",
		Short:       "Delete a custom field (DELETE /fields/{id})",
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
