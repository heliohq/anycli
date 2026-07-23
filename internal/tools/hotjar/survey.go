package hotjar

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newSurveyCmd groups the survey read surface — the core of what an analytics
// teammate does with Hotjar: enumerate a site's surveys, inspect one, and
// export its responses (voice-of-customer feedback). All three are read-only.
func (s *Service) newSurveyCmd(creds clientCreds) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "survey",
		Short: "List surveys and export survey responses",
	}
	cmd.AddCommand(
		s.newSurveyListCmd(creds),
		s.newSurveyGetCmd(creds),
		s.newSurveyResponsesCmd(creds),
	)
	return cmd
}

func (s *Service) newSurveyListCmd(creds clientCreds) *cobra.Command {
	var site, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List surveys for a site",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), creds,
				fmt.Sprintf("/v1/sites/%s/surveys", url.PathEscape(site)),
				pageQuery(cursor, limit))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "Hotjar site id (required)")
	registerPageFlags(cmd, &cursor, &limit)
	_ = cmd.MarkFlagRequired("site")
	return cmd
}

func (s *Service) newSurveyGetCmd(creds clientCreds) *cobra.Command {
	var site, survey string
	var withQuestions bool
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one survey's detail",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if withQuestions {
				q.Set("with_questions", "true")
			}
			body, err := s.get(cmd.Context(), creds,
				fmt.Sprintf("/v1/sites/%s/surveys/%s", url.PathEscape(site), url.PathEscape(survey)), q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "Hotjar site id (required)")
	cmd.Flags().StringVar(&survey, "survey", "", "survey id (required)")
	cmd.Flags().BoolVar(&withQuestions, "with-questions", false, "include question metadata")
	_ = cmd.MarkFlagRequired("site")
	_ = cmd.MarkFlagRequired("survey")
	return cmd
}

func (s *Service) newSurveyResponsesCmd(creds clientCreds) *cobra.Command {
	var site, survey, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:         "responses",
		Short:       "Export a survey's responses (newest first, cursor-paginated)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), creds,
				fmt.Sprintf("/v1/sites/%s/surveys/%s/responses", url.PathEscape(site), url.PathEscape(survey)),
				pageQuery(cursor, limit))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "Hotjar site id (required)")
	cmd.Flags().StringVar(&survey, "survey", "", "survey id (required)")
	registerPageFlags(cmd, &cursor, &limit)
	_ = cmd.MarkFlagRequired("site")
	_ = cmd.MarkFlagRequired("survey")
	return cmd
}

// registerPageFlags wires Hotjar's cursor pagination onto a list command.
func registerPageFlags(cmd *cobra.Command, cursor *string, limit *int) {
	cmd.Flags().StringVar(cursor, "cursor", "", "pagination cursor (from a prior response's next_cursor)")
	cmd.Flags().IntVar(limit, "limit", 0, "max items to return (0 = server default)")
}

// pageQuery builds the cursor/limit query parameters shared by the list verbs.
func pageQuery(cursor string, limit int) url.Values {
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return q
}
