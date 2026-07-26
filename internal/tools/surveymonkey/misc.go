package surveymonkey

import (
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newCollectorListCmd(token string) *cobra.Command {
	var survey string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the collectors that gathered a survey's responses",
		Long: "`--survey` is required. A collector is one distribution channel — a web\n" +
			"link, an email invitation — and this says how responses were gathered when\n" +
			"the answers themselves are paywalled. `--page` and `--per-page` page it.\n" +
			"Free-plan usable.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFlag("survey", survey); err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), token, "/surveys/"+url.PathEscape(survey)+"/collectors", pagingValues(page, perPage))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&survey, "survey", "", "survey id")
	addPagingFlags(cmd, &page, &perPage)
	return cmd
}

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the connected SurveyMonkey user (identity, team, plan)",
		Long: "Names the account behind the connection and reports its plan, which is what\n" +
			"decides whether `response bulk` and `response get` can return answers at\n" +
			"all. Worth one call before promising an analysis that depends on them.\n" +
			"Takes no flags.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), token, "/users/me", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newFetchCmd is a generic read escape hatch for any v3 GET endpoint not modeled
// as a subcommand (notion `fetch` precedent). The --path value is v3-relative;
// a leading slash and/or an explicit "v3/" prefix are tolerated and normalized.
func (s *Service) newFetchCmd(token string) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "GET any v3 endpoint by path (read-only escape hatch)",
		Long: "For v3 endpoints this tool does not model — survey rollups, trends,\n" +
			"templates. `--path` is v3-relative (`surveys/123/rollups`); a leading\n" +
			"slash and an explicit `v3/` prefix are both tolerated and stripped. GET\n" +
			"only, so it cannot become a write, and it takes no query flags: anything\n" +
			"the endpoint needs has to be spelled into `--path` itself. The paid-plan\n" +
			"gate on answer data still applies to whatever endpoint is reached.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFlag("path", path); err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), token, "/"+normalizeFetchPath(path), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "v3-relative path, e.g. surveys/123/rollups")
	return cmd
}

// normalizeFetchPath strips a leading slash and an optional "v3/" prefix so both
// "surveys/1", "/surveys/1", and "/v3/surveys/1" resolve to the same v3 path.
func normalizeFetchPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "v3/")
	return p
}
