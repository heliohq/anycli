package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newFormCmd builds the `mailerlite form` command tree — signup-form inventory
// and who came in through which form.
func (s *Service) newFormCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "form", Short: "Forms (list, get, update, delete, subscribers)"}
	cmd.AddCommand(
		s.newFormListCmd(token),
		s.newFormGetCmd(token),
		s.newFormUpdateCmd(token),
		s.newFormDeleteCmd(token),
		s.newFormSubscribersCmd(token),
	)
	return cmd
}

func (s *Service) newFormListCmd(token string) *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use:   "list <type>",
		Short: "List forms of a type (GET /forms/{type}); type is popup|embedded|promotion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormType(args[0]); err != nil {
				return err
			}
			q := url.Values{}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(args[0]), q, nil)
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

// validateFormType guards the type path segment against the closed set the API
// accepts, so a typo fails as a usage error rather than a 404.
func validateFormType(t string) error {
	switch t {
	case "popup", "embedded", "promotion":
		return nil
	default:
		return &usageError{msg: "form type must be one of popup|embedded|promotion, got " + t}
	}
}

func (s *Service) newFormGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a form (GET /forms/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newFormUpdateCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Rename a form (PUT /forms/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/forms/"+url.PathEscape(args[0]), nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new form name (required)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newFormDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a form (DELETE /forms/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/forms/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newFormSubscribersCmd(token string) *cobra.Command {
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "subscribers <id>",
		Short: "List subscribers who signed up through a form (GET /forms/{id}/subscribers)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			setLimitCursor(cmd, q, limit, cursor)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(args[0])+"/subscribers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	return cmd
}
