package posthog

import (
	"github.com/spf13/cobra"
)

// newPersonCmd groups person lookup (list with search, get by id).
func (s *Service) newPersonCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "person", Short: "Persons (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List/search persons (GET /api/projects/<id>/persons/)", "/persons/", true),
		s.newProjectGetCmd(token, "get", "Get a person (GET /api/projects/<id>/persons/<id>/)", "/persons/"),
	)
	return cmd
}

// newCohortCmd groups cohort read access.
func (s *Service) newCohortCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "cohort", Short: "Cohorts (list)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List cohorts (GET /api/projects/<id>/cohorts/)", "/cohorts/", false),
	)
	return cmd
}
