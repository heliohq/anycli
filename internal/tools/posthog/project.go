package posthog

import (
	"github.com/spf13/cobra"
)

// newProjectCmd groups project discovery. Project list is org-scoped
// (/api/projects/), the entry point an agent uses to discover the --project id
// every other command needs.
func (s *Service) newProjectCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Projects (list)"}
	cmd.AddCommand(s.newProjectListSubcmd(token))
	return cmd
}

func (s *Service) newProjectListSubcmd(token string) *cobra.Command {
	var lp listParams
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects the token can access (GET /api/projects/)",
		Long: "The `id` on each row is what every other command's --project expects; the\n" +
			"project `name` is not accepted anywhere. This is the only read that works\n" +
			"before a project id is known, because it is scoped to the organization\n" +
			"rather than to a project.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, "GET", "/api/projects/", lp.values(false), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lp.register(cmd, false)
	return cmd
}
