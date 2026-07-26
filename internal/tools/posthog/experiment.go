package posthog

import (
	"github.com/spf13/cobra"
)

// The experiment Longs live here because both leaves come from the shared
// project-scoped constructors.
const (
	longExperimentList = "Every PostHog experiment is bound to a feature flag, and it is the flag that\n" +
		"performs variant assignment. The rows carry that flag's key and id, so\n" +
		"halting an experiment's exposure is a `flag toggle` on the linked flag\n" +
		"rather than anything under `experiment`."

	longExperimentGet = "Returns one experiment with its linked feature flag, its variant definitions\n" +
		"and the metric it is judged on. Statistical results sit behind a separate\n" +
		"PostHog endpoint this tool does not wrap, so significance has to come from\n" +
		"`query run` over the exposure events. --id is the numeric experiment id."
)

// newExperimentCmd groups experiment (A/B test) read access.
func (s *Service) newExperimentCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "experiment", Short: "Experiments (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List experiments (GET /api/projects/<id>/experiments/)", longExperimentList, "/experiments/", false),
		s.newProjectGetCmd(token, "get", "Get an experiment (GET /api/projects/<id>/experiments/<id>/)", longExperimentGet, "/experiments/"),
	)
	return cmd
}
