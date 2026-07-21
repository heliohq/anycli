package formstack

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newFormCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "form", Short: "Forms (list, get, fields, create, copy, delete)"}
	cmd.AddCommand(
		s.newFormListCmd(token),
		s.newFormGetCmd(token),
		s.newFormFieldsCmd(token),
		s.newFormCreateCmd(token),
		s.newFormCopyCmd(token),
		s.newFormDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newFormListCmd(token string) *cobra.Command {
	var search, folder, sort string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List forms (GET /form.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if search != "" {
				q.Set("search", search)
			}
			if folder != "" {
				q.Set("folder", folder)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			if cmd.Flags().Changed("page") {
				q.Set("page", itoa(page))
			}
			if cmd.Flags().Changed("per-page") {
				q.Set("per_page", itoa(perPage))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form.json", q, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "search forms by name")
	cmd.Flags().StringVar(&folder, "folder", "", "filter by folder id")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order: id|name-asc|desc")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().IntVar(&perPage, "per-page", 25, "results per page (max 100)")
	return cmd
}

func (s *Service) newFormGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <form-id>",
		Short: "Get a form (GET /form/{id}.json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newFormFieldsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fields <form-id>",
		Short: "List a form's fields (GET /form/{id}/field.json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form/"+url.PathEscape(args[0])+"/field.json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newFormCreateCmd(token string) *cobra.Command {
	var name, folder string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a form (POST /form.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name}
			if folder != "" {
				body["folder"] = folder
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/form.json", nil, body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "form name")
	cmd.Flags().StringVar(&folder, "folder", "", "folder id to create the form in")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newFormCopyCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copy <form-id>",
		Short: "Copy a form (POST /form/{id}/copy.json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/form/"+url.PathEscape(args[0])+"/copy.json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newFormDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <form-id>",
		Short: "Delete a form (DELETE /form/{id}.json); soft delete per the API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/form/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
