package tally

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newSubmissionCmd(token string) *cobra.Command {
	cmd := newGroupCmd("submission", "Form submissions (list, get)")
	cmd.AddCommand(
		s.newSubmissionListCmd(token),
		s.newSubmissionGetCmd(token),
	)
	return cmd
}

func (s *Service) newSubmissionListCmd(token string) *cobra.Command {
	var form, filter, startDate, endDate, afterID string
	var page, limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a form's submissions (GET /forms/{formId}/submissions)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filter != "" {
				if err := oneOfFlag("filter", filter, []string{"all", "completed", "partial"}); err != nil {
					return err
				}
			}
			q := url.Values{}
			if cmd.Flags().Changed("page") {
				q.Set("page", strconv.Itoa(page))
			}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", strconv.Itoa(limit))
			}
			setIf(q, "filter", filter)
			setIf(q, "startDate", startDate)
			setIf(q, "endDate", endDate)
			setIf(q, "afterId", afterID)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(form)+"/submissions", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "form id")
	cmd.Flags().StringVar(&filter, "filter", "", "filter: all|completed|partial")
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size")
	cmd.Flags().StringVar(&afterID, "after-id", "", "return submissions after this submission id (cursor)")
	cmd.Flags().StringVar(&startDate, "start-date", "", "ISO-8601 lower bound")
	cmd.Flags().StringVar(&endDate, "end-date", "", "ISO-8601 upper bound")
	_ = cmd.MarkFlagRequired("form")
	return cmd
}

func (s *Service) newSubmissionGetCmd(token string) *cobra.Command {
	var form, submission string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one submission (GET /forms/{formId}/submissions/{submissionId})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet,
				"/forms/"+url.PathEscape(form)+"/submissions/"+url.PathEscape(submission), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "form id")
	cmd.Flags().StringVar(&submission, "submission", "", "submission id")
	_ = cmd.MarkFlagRequired("form")
	_ = cmd.MarkFlagRequired("submission")
	return cmd
}
