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
		Use:         "list",
		Short:       "List the collectors that gathered a survey's responses",
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
		Use:         "me",
		Short:       "Show the connected SurveyMonkey user (identity, team, plan)",
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
		Use:         "fetch",
		Short:       "GET any v3 endpoint by path (read-only escape hatch)",
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
