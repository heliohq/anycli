package posthog

import (
	"github.com/spf13/cobra"
)

// longEventDefinitionList and longPropertyDefinitionList sit next to their
// groups because both leaves are built by the shared project-list constructor,
// which cannot carry per-command prose of its own.
const (
	longEventDefinitionList = "Lists the event names the project has actually ingested, which is the\n" +
		"source of truth for what `query run` can select on. Check here first when a\n" +
		"HogQL query returns zero rows: a misspelled event name yields an empty\n" +
		"result set, not an error. `--search` filters by name."

	longPropertyDefinitionList = "The companion to `event-definition list`: the property keys the project has\n" +
		"ingested, which is what `properties.<key>` in a HogQL filter can reference.\n" +
		"Each row carries the type PostHog inferred, so a numeric comparison against\n" +
		"a property stored as a string can be caught before the query is written."
)

// newEventDefinitionCmd lists the project's tracked event definitions — a HogQL
// authoring aid (what event names exist to query).
func (s *Service) newEventDefinitionCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "event-definition", Short: "Event definitions (list)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List event definitions (GET /api/projects/<id>/event_definitions/)", longEventDefinitionList, "/event_definitions/", true),
	)
	return cmd
}

// newPropertyDefinitionCmd lists the project's tracked property definitions —
// the companion HogQL authoring aid (what properties exist to filter on).
func (s *Service) newPropertyDefinitionCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "property-definition", Short: "Property definitions (list)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List property definitions (GET /api/projects/<id>/property_definitions/)", longPropertyDefinitionList, "/property_definitions/", true),
	)
	return cmd
}
