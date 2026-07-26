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
		Use:   "list",
		Short: "List the account's forms (GET /user/forms)",
		Long: "Each row's `id` is the `<formID>` every other form command takes, alongside\n" +
			"the title, `status` and a running count of submissions received. Disabled,\n" +
			"archived and deleted forms come back mixed in with live ones, so\n" +
			"`--filter '{\"status\":\"ENABLED\"}'` is usually what is wanted rather than the\n" +
			"raw list.",
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
		Use:   "get <formID>",
		Short: "Get one form's details (GET /form/{id})",
		Long: "Metadata only: title, `status`, created and updated timestamps, submission\n" +
			"counts and the form's public URL. It does NOT include the form's fields — the\n" +
			"questions a submission has to answer come from `form questions`, and writing\n" +
			"without reading those first is the main way a submission ends up empty.",
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
		Use:   "questions <formID>",
		Short: "List a form's questions and their qids (GET /form/{id}/questions)",
		Long: "The prerequisite for every submission read or write. The response is keyed by\n" +
			"qid, and each entry carries the question's `type`, `name` and `text`. Composite\n" +
			"types — `control_fullname`, `control_address` and their relatives — are not\n" +
			"written as one value: they take the `qid:subfield=value` form that\n" +
			"`submission create` and `submission edit` accept, and the subfield names come\n" +
			"from the question's own definition here.",
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
		Use:   "submissions <formID>",
		Short: "List one form's submissions (GET /form/{id}/submissions)",
		Long: "Each response's answers are keyed by qid rather than by label, so pair this\n" +
			"with `form questions <formID>` to read the values meaningfully. `--filter`\n" +
			"takes a Jotform JSON filter object and is the practical way to bound a busy\n" +
			"form by date or status. For responses across every form at once, use\n" +
			"`submission list` instead.",
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
