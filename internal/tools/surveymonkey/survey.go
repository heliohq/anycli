package surveymonkey

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// addPagingFlags registers the shared 1-based page / per_page paging flags. A
// value of 0 means "unset" — no param is sent, so the provider default applies.
func addPagingFlags(cmd *cobra.Command, page, perPage *int) {
	cmd.Flags().IntVar(page, "page", 0, "1-based page number (provider default when unset)")
	cmd.Flags().IntVar(perPage, "per-page", 0, "results per page (provider default when unset)")
}

// pagingValues builds query params for the paging flags, omitting any left at
// the unset sentinel (0).
func pagingValues(page, perPage int) url.Values {
	v := url.Values{}
	if page > 0 {
		v.Set("page", strconv.Itoa(page))
	}
	if perPage > 0 {
		v.Set("per_page", strconv.Itoa(perPage))
	}
	return v
}

func (s *Service) newSurveyListCmd(token string) *cobra.Command {
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List surveys in the connected account (one page)",
		Long: "The discovery step: returns id and title per survey, which is what every\n" +
			"other command's `--survey` or `--id` needs. Metadata such as\n" +
			"`response_count` is NOT here — that comes from `survey get` on one id.\n" +
			"`--page` and `--per-page` are 1-based and left to SurveyMonkey's defaults\n" +
			"when unset; compare `total` in the envelope against what came back before\n" +
			"assuming this is all of them.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), token, "/surveys", pagingValues(page, perPage))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	addPagingFlags(cmd, &page, &perPage)
	return cmd
}

func (s *Service) newSurveyGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a survey's metadata (title, response_count, dates)",
		Long: "`--id` is a required flag. `response_count` here is the cheapest answer to\n" +
			"\"how many people replied\" — one call, no paging, and it works on a free\n" +
			"plan. For a count of only completed responses use `response list\n" +
			"--status completed` and read `total` from its envelope. This returns no\n" +
			"questions; that is `survey details`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFlag("id", id); err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), token, "/surveys/"+url.PathEscape(id), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "survey id")
	return cmd
}

func (s *Service) newSurveyDetailsCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "details",
		Short: "Get a survey's full structure (pages, questions, answer-option ids)",
		Long: "`--id` is required. This is the decoder ring: responses cite question ids\n" +
			"and answer-option ids, so without this the answers from `response bulk` or\n" +
			"`response get` cannot be turned into text. Fetch it ONCE per survey and\n" +
			"reuse it across every response. It is free-plan usable even when the\n" +
			"answers themselves are not.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFlag("id", id); err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), token, "/surveys/"+url.PathEscape(id)+"/details", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "survey id")
	return cmd
}
