package outreach

import "github.com/spf13/cobra"

var (
	userResource     = resource{path: "users", typ: "user"}
	templateResource = resource{path: "templates", typ: "template"}
	stageResource    = resource{path: "stages", typ: "stage"}
	personaResource  = resource{path: "personas", typ: "persona"}
)

// newUserCmd builds the user group — owner resolution / assignment.
func (s *Service) newUserCmd(token string) *cobra.Command {
	group := newGroupCmd("user", "List and look up users")
	group.AddCommand(
		s.newListCmd(token, userResource),
		s.newGetCmd(token, userResource),
	)
	return group
}

// newTemplateCmd builds the template group — inspect email templates referenced
// by sequence steps.
func (s *Service) newTemplateCmd(token string) *cobra.Command {
	group := newGroupCmd("template", "List and look up email templates")
	group.AddCommand(
		s.newListCmd(token, templateResource),
		s.newGetCmd(token, templateResource),
	)
	return group
}

// newStageCmd builds the stage group — ids needed when creating/updating prospects.
func (s *Service) newStageCmd(token string) *cobra.Command {
	group := newGroupCmd("stage", "List prospect stages")
	group.AddCommand(s.newListCmd(token, stageResource))
	return group
}

// newPersonaCmd builds the persona group — ids needed when creating/updating prospects.
func (s *Service) newPersonaCmd(token string) *cobra.Command {
	group := newGroupCmd("persona", "List prospect personas")
	group.AddCommand(s.newListCmd(token, personaResource))
	return group
}
