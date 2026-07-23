package posthog

import (
	"github.com/spf13/cobra"
)

// newEventDefinitionCmd lists the project's tracked event definitions — a HogQL
// authoring aid (what event names exist to query).
func (s *Service) newEventDefinitionCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "event-definition", Short: "Event definitions (list)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List event definitions (GET /api/projects/<id>/event_definitions/)", "/event_definitions/", true),
	)
	return cmd
}

// newPropertyDefinitionCmd lists the project's tracked property definitions —
// the companion HogQL authoring aid (what properties exist to filter on).
func (s *Service) newPropertyDefinitionCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "property-definition", Short: "Property definitions (list)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List property definitions (GET /api/projects/<id>/property_definitions/)", "/property_definitions/", true),
	)
	return cmd
}
