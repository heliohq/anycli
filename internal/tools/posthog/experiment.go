package posthog

import (
	"github.com/spf13/cobra"
)

// newExperimentCmd groups experiment (A/B test) read access.
func (s *Service) newExperimentCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "experiment", Short: "Experiments (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List experiments (GET /api/projects/<id>/experiments/)", "/experiments/", false),
		s.newProjectGetCmd(token, "get", "Get an experiment (GET /api/projects/<id>/experiments/<id>/)", "/experiments/"),
	)
	return cmd
}
