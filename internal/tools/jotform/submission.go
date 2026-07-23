package jotform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newSubmissionCmd(key string) *cobra.Command {
	cmd := newGroupCmd("submission", "Read, create, edit, and delete submissions")
	cmd.AddCommand(
		s.newSubmissionListCmd(key),
		s.newSubmissionGetCmd(key),
		s.newSubmissionCreateCmd(key),
		s.newSubmissionEditCmd(key),
		s.newSubmissionDeleteCmd(key),
	)
	return cmd
}

func (s *Service) newSubmissionListCmd(key string) *cobra.Command {
	var params listParams
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List submissions across all the account's forms (GET /user/submissions)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			params.apply(q)
			body, err := s.get(cmd.Context(), key, "/user/submissions", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, &params)
	return cmd
}

func (s *Service) newSubmissionGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <submissionID>",
		Short:       "Get one submission (GET /submission/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.get(cmd.Context(), key, "/submission/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newSubmissionCreateCmd(key string) *cobra.Command {
	var fields []string
	cmd := &cobra.Command{
		Use:         "create <formID> --field <qid>=<value> [--field <qid:subfield>=<value> ...]",
		Short:       "Create a submission on a form (POST /form/{id}/submissions; Full Access key)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := encodeFields(fields)
			if err != nil {
				return err
			}
			body, err := s.postForm(cmd.Context(), key, "/form/"+url.PathEscape(args[0])+"/submissions", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "answer as qid=value (or qid:subfield=value for composite fields); repeatable")
	return cmd
}

func (s *Service) newSubmissionEditCmd(key string) *cobra.Command {
	var fields []string
	cmd := &cobra.Command{
		Use:         "edit <submissionID> --field <qid>=<value> [...]",
		Short:       "Edit an existing submission's answers (POST /submission/{id}; Full Access key)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := encodeFields(fields)
			if err != nil {
				return err
			}
			body, err := s.postForm(cmd.Context(), key, "/submission/"+url.PathEscape(args[0]), form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "answer as qid=value (or qid:subfield=value for composite fields); repeatable")
	return cmd
}

func (s *Service) newSubmissionDeleteCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <submissionID>",
		Short:       "Delete a submission (DELETE /submission/{id}; Full Access key)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodDelete, "/submission/"+url.PathEscape(args[0]), nil, nil, "")
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// postForm sends url.Values as an application/x-www-form-urlencoded body, the
// shape Jotform's submission create/edit endpoints expect.
func (s *Service) postForm(ctx context.Context, key, path string, form url.Values) ([]byte, error) {
	return s.call(ctx, key, http.MethodPost, path, nil, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
}

// encodeFields turns repeated --field values into Jotform's submission form
// keys: "qid=value" → submission[qid]=value, and "qid:subfield=value" →
// submission[qid][subfield]=value (composite fields like name/address). The
// split is on the FIRST '=' only, so a value may itself contain '='.
func encodeFields(fields []string) (url.Values, error) {
	form := url.Values{}
	for _, raw := range fields {
		eq := strings.IndexByte(raw, '=')
		if eq < 0 {
			return nil, &usageError{msg: fmt.Sprintf("--field %q must be qid=value or qid:subfield=value", raw)}
		}
		lhs, value := raw[:eq], raw[eq+1:]
		qid, subfield, hasSub := strings.Cut(lhs, ":")
		qid = strings.TrimSpace(qid)
		if qid == "" {
			return nil, &usageError{msg: fmt.Sprintf("--field %q has an empty qid", raw)}
		}
		if hasSub {
			subfield = strings.TrimSpace(subfield)
			if subfield == "" {
				return nil, &usageError{msg: fmt.Sprintf("--field %q has an empty subfield", raw)}
			}
			form.Set(fmt.Sprintf("submission[%s][%s]", qid, subfield), value)
			continue
		}
		form.Set(fmt.Sprintf("submission[%s]", qid), value)
	}
	return form, nil
}
