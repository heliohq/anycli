package tally

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newFormCmd(token string) *cobra.Command {
	cmd := newGroupCmd("form", "Forms (list, get, questions, create, update, delete)")
	cmd.AddCommand(
		s.newFormListCmd(token),
		s.newFormGetCmd(token),
		s.newFormQuestionsCmd(token),
		s.newFormCreateCmd(token),
		s.newFormUpdateCmd(token),
		s.newFormDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newFormListCmd(token string) *cobra.Command {
	var workspaces []string
	var page, limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List forms (GET /forms)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if cmd.Flags().Changed("page") {
				q.Set("page", strconv.Itoa(page))
			}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", strconv.Itoa(limit))
			}
			for _, ws := range workspaces {
				q.Add("workspaceIds", ws)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/forms", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&workspaces, "workspace", nil, "filter by workspace id (repeatable)")
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size")
	return cmd
}

func (s *Service) newFormGetCmd(token string) *cobra.Command {
	var form string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a form (GET /forms/{formId})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(form), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "form id")
	_ = cmd.MarkFlagRequired("form")
	return cmd
}

func (s *Service) newFormQuestionsCmd(token string) *cobra.Command {
	var form string
	cmd := &cobra.Command{
		Use:         "questions",
		Short:       "List a form's questions (GET /forms/{formId}/questions)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(form)+"/questions", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "form id")
	_ = cmd.MarkFlagRequired("form")
	return cmd
}

func (s *Service) newFormCreateCmd(token string) *cobra.Command {
	var file string
	var stdin bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a form (POST /forms)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.readBody(file, stdin)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/forms", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	bodyFlags(cmd, &file, &stdin)
	return cmd
}

func (s *Service) newFormUpdateCmd(token string) *cobra.Command {
	var form, file string
	var stdin bool
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a form (PATCH /forms/{formId})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.readBody(file, stdin)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/forms/"+url.PathEscape(form), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "form id")
	bodyFlags(cmd, &file, &stdin)
	_ = cmd.MarkFlagRequired("form")
	return cmd
}

func (s *Service) newFormDeleteCmd(token string) *cobra.Command {
	var form string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a form (DELETE /forms/{formId})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/forms/"+url.PathEscape(form), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "form id")
	_ = cmd.MarkFlagRequired("form")
	return cmd
}
