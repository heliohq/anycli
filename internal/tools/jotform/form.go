package jotform

import (
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newFormCmd(key string) *cobra.Command {
	cmd := newGroupCmd("form", "List and inspect forms")
	cmd.AddCommand(
		s.newFormListCmd(key),
		s.newFormGetCmd(key),
		s.newFormQuestionsCmd(key),
		s.newFormSubmissionsCmd(key),
	)
	return cmd
}

func (s *Service) newFormListCmd(key string) *cobra.Command {
	var params listParams
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List the account's forms (GET /user/forms)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			params.apply(q)
			body, err := s.get(cmd.Context(), key, "/user/forms", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, &params)
	return cmd
}

func (s *Service) newFormGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <formID>",
		Short:       "Get one form's details (GET /form/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.get(cmd.Context(), key, "/form/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newFormQuestionsCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "questions <formID>",
		Short:       "List a form's questions and their qids (GET /form/{id}/questions)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.get(cmd.Context(), key, "/form/"+url.PathEscape(args[0])+"/questions", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newFormSubmissionsCmd(key string) *cobra.Command {
	var params listParams
	cmd := &cobra.Command{
		Use:         "submissions <formID>",
		Short:       "List one form's submissions (GET /form/{id}/submissions)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			params.apply(q)
			body, err := s.get(cmd.Context(), key, "/form/"+url.PathEscape(args[0])+"/submissions", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, &params)
	return cmd
}
