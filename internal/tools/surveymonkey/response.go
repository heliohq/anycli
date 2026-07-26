package surveymonkey

import (
	"net/url"

	"github.com/spf13/cobra"
)

// responseFilterFlags holds the optional filters shared by the response list /
// bulk commands. All are omitted from the query when empty.
type responseFilterFlags struct {
	status          string
	startModifiedAt string
	endModifiedAt   string
	startCreatedAt  string
	endCreatedAt    string
}

func addResponseFilterFlags(cmd *cobra.Command, f *responseFilterFlags) {
	cmd.Flags().StringVar(&f.status, "status", "", "filter by response status (completed, partial, overquota, disqualified)")
	cmd.Flags().StringVar(&f.startModifiedAt, "start-modified-at", "", "only responses modified at/after this timestamp (YYYY-MM-DDTHH:MM:SS)")
	cmd.Flags().StringVar(&f.endModifiedAt, "end-modified-at", "", "only responses modified at/before this timestamp")
	cmd.Flags().StringVar(&f.startCreatedAt, "start-created-at", "", "only responses created at/after this timestamp")
	cmd.Flags().StringVar(&f.endCreatedAt, "end-created-at", "", "only responses created at/before this timestamp")
}

func (f responseFilterFlags) apply(v url.Values) {
	setIf := func(key, val string) {
		if val != "" {
			v.Set(key, val)
		}
	}
	setIf("status", f.status)
	setIf("start_modified_at", f.startModifiedAt)
	setIf("end_modified_at", f.endModifiedAt)
	setIf("start_created_at", f.startCreatedAt)
	setIf("end_created_at", f.endCreatedAt)
}

// newResponseListCmd wraps the non-bulk GET /surveys/{id}/responses metadata
// list: paginated response ids/hrefs (no answer content), filterable by
// status/date, free-plan usable under responses_read.
func (s *Service) newResponseListCmd(token string) *cobra.Command {
	var survey string
	var page, perPage int
	var filters responseFilterFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List response metadata for a survey (ids/hrefs; no answers; free-plan usable)",
		Long: "`--survey` is required. Deliberately carries no answer content, which is\n" +
			"why it works without the paid permission that `response bulk` needs. Its\n" +
			"real use beyond ids is COUNTING: apply `--status` (completed, partial,\n" +
			"overquota, disqualified) or the `--start-created-at` / `--end-created-at` /\n" +
			"`--start-modified-at` / `--end-modified-at` timestamps\n" +
			"(YYYY-MM-DDTHH:MM:SS) and read `total` from the envelope for the filtered\n" +
			"count, without paging through anything.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFlag("survey", survey); err != nil {
				return err
			}
			v := pagingValues(page, perPage)
			filters.apply(v)
			body, err := s.get(cmd.Context(), token, "/surveys/"+url.PathEscape(survey)+"/responses", v)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&survey, "survey", "", "survey id")
	addPagingFlags(cmd, &page, &perPage)
	addResponseFilterFlags(cmd, &filters)
	return cmd
}

// newResponseBulkCmd wraps GET /surveys/{id}/responses/bulk: full expanded
// responses including answers to every question. Requires the paid
// responses_read_detail scope; on a free/unentitled connection SurveyMonkey
// returns 1014 (optional paid scope ungranted) or 1015 (plan gate), both mapped
// to a paid-plan message.
func (s *Service) newResponseBulkCmd(token string) *cobra.Command {
	var survey string
	var page, perPage int
	var filters responseFilterFlags
	cmd := &cobra.Command{
		Use:   "bulk",
		Short: "Read full responses with answers for a survey (needs paid responses_read_detail)",
		Long: "`--survey` is required. This is the command for analysing what people\n" +
			"actually said: one page returns many complete responses, so it beats\n" +
			"looping `response get` by a wide margin. It takes the same `--status` and\n" +
			"date filters as `response list`, plus `--page` / `--per-page`.\n" +
			"\n" +
			"It needs the paid `responses_read_detail` permission and fails on a free\n" +
			"plan with an explicit message — treat that as final rather than retrying.\n" +
			"Answers arrive as question and answer-option ids, so pair it with\n" +
			"`survey details`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFlag("survey", survey); err != nil {
				return err
			}
			v := pagingValues(page, perPage)
			filters.apply(v)
			body, err := s.get(cmd.Context(), token, "/surveys/"+url.PathEscape(survey)+"/responses/bulk", v)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&survey, "survey", "", "survey id")
	addPagingFlags(cmd, &page, &perPage)
	addResponseFilterFlags(cmd, &filters)
	return cmd
}

// newResponseGetCmd wraps GET /surveys/{id}/responses/{rid}/details: one full
// response with all answers. Same paid responses_read_detail requirement as bulk.
func (s *Service) newResponseGetCmd(token string) *cobra.Command {
	var survey, id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Read one full response with answers (needs paid responses_read_detail)",
		Long: "Both `--survey` and `--id` are required; a response id does not address\n" +
			"anything on its own. Same paid `responses_read_detail` gate as\n" +
			"`response bulk`, and no filters or paging. Worth calling only for one\n" +
			"already-identified respondent — for a whole survey `response bulk` returns\n" +
			"the same content many at a time.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFlag("survey", survey); err != nil {
				return err
			}
			if err := requireFlag("id", id); err != nil {
				return err
			}
			path := "/surveys/" + url.PathEscape(survey) + "/responses/" + url.PathEscape(id) + "/details"
			body, err := s.get(cmd.Context(), token, path, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&survey, "survey", "", "survey id")
	cmd.Flags().StringVar(&id, "id", "", "response id")
	return cmd
}
