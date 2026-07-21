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
		Args:  cobra.NoArgs,
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
