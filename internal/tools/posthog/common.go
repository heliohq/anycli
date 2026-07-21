package posthog

import (
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// requireProject rejects a project-scoped command that was given no --project.
func requireProject(project string) error {
	if project == "" {
		return &usageError{msg: "--project is required (run `project list` to discover project ids)"}
	}
	return nil
}

// requireFlag rejects an empty required flag value.
func requireFlag(name, value string) error {
	if value == "" {
		return &usageError{msg: "--" + name + " is required"}
	}
	return nil
}

// projectPath builds a project-scoped API path: /api/projects/<id><suffix>.
func projectPath(project, suffix string) string {
	return "/api/projects/" + url.PathEscape(project) + suffix
}

// listParams holds the shared DRF-style paging/search query flags PostHog list
// endpoints accept.
type listParams struct {
	limit  int
	offset int
	search string
}

// register wires --limit / --offset (and --search when the endpoint supports
// it) onto a list command.
func (p *listParams) register(cmd *cobra.Command, withSearch bool) {
	cmd.Flags().IntVar(&p.limit, "limit", 0, "max rows to return (0 = provider default)")
	cmd.Flags().IntVar(&p.offset, "offset", 0, "row offset for pagination")
	if withSearch {
		cmd.Flags().StringVar(&p.search, "search", "", "search filter")
	}
}

// values renders the paging/search flags into a query value set.
func (p listParams) values(withSearch bool) url.Values {
	q := url.Values{}
	if p.limit > 0 {
		q.Set("limit", strconv.Itoa(p.limit))
	}
	if p.offset > 0 {
		q.Set("offset", strconv.Itoa(p.offset))
	}
	if withSearch && p.search != "" {
		q.Set("search", p.search)
	}
	return q
}

// readFileOrStdin reads a flag value that names a file, or stdin when "-".
func readFileOrStdin(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	return os.ReadFile(path)
}

// newProjectListCmd builds a project-scoped list command keyed on a path
// suffix (e.g. "/insights/"), auto-wiring --project and the paging flags.
func (s *Service) newProjectListCmd(token, use, short, suffix string, withSearch bool) *cobra.Command {
	var project string
	var lp listParams
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireProject(project); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, "GET", projectPath(project, suffix), lp.values(withSearch), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id (required)")
	lp.register(cmd, withSearch)
	return cmd
}

// newProjectGetCmd builds a project-scoped get-by-id command keyed on a path
// prefix (e.g. "/insights/"), auto-wiring --project and --id.
func (s *Service) newProjectGetCmd(token, use, short, prefix string) *cobra.Command {
	var project, id string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireProject(project); err != nil {
				return err
			}
			if err := requireFlag("id", id); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, "GET", projectPath(project, prefix+url.PathEscape(id)+"/"), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id (required)")
	cmd.Flags().StringVar(&id, "id", "", "object id (required)")
	return cmd
}
