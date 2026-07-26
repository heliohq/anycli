package posthog

import (
	"github.com/spf13/cobra"
)

// The person and cohort Longs live here because all three leaves come from the
// shared project-scoped constructors.
const (
	longPersonList = "`--search` matches distinct ids, email and person properties, which is how\n" +
		"an email address or an application user id is turned into the numeric\n" +
		"person id the rest of this surface takes. A person row carries merged\n" +
		"identities, so one person can answer to several distinct ids."

	longPersonGet = "Takes the numeric person id from `person list`, not a distinct id and not an\n" +
		"email. Returns the person's current merged properties only; the events they\n" +
		"produced are read with `query run` filtering on `person_id`."

	longCohortList = "Cohorts are saved person filters, either static snapshots or dynamically\n" +
		"re-evaluated. This returns their definitions and counts. There is no\n" +
		"command that enumerates a cohort's members — reach for `query run` when the\n" +
		"people themselves are needed."
)

// newPersonCmd groups person lookup (list with search, get by id).
func (s *Service) newPersonCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "person", Short: "Persons (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List/search persons (GET /api/projects/<id>/persons/)", longPersonList, "/persons/", true),
		s.newProjectGetCmd(token, "get", "Get a person (GET /api/projects/<id>/persons/<id>/)", longPersonGet, "/persons/"),
	)
	return cmd
}

// newCohortCmd groups cohort read access.
func (s *Service) newCohortCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "cohort", Short: "Cohorts (list)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List cohorts (GET /api/projects/<id>/cohorts/)", longCohortList, "/cohorts/", false),
	)
	return cmd
}
