package outreach

import "github.com/spf13/cobra"

// The reference-resource Longs live here because all six leaves come from the
// shared list/get constructors.
const (
	longUserList = "The org's seats. Each `id` is what --owner-id takes on the prospect,\n" +
		"account and task lists, and what an `owner_id` in any response refers to.\n" +
		"There is no email filter here, so page and match locally."

	longUserGet = "One Outreach seat: the person an `owner_id` points at. Use it to turn an\n" +
		"id from some other response into a name before reporting it."

	longTemplateList = "Email templates available to sequence steps. The bodies are not here —\n" +
		"`template get` returns one, which is how to see what a cadence will\n" +
		"actually put in front of a prospect."

	longTemplateGet = "One template including its body, which is the text a sequence step sends.\n" +
		"Reading it before `enrollment add` is the only way to know what the prospect\n" +
		"receives."

	longStageList = "Stages are the org's own prospect-pipeline labels. Their ids are what\n" +
		"`prospect create --stage-id` and `prospect list --stage-id` take. The set is\n" +
		"defined in Outreach and nothing here creates one."

	longPersonaList = "Personas are the org's buyer-role labels. This tool reads them but has no\n" +
		"flag that assigns one to a prospect, so the ids are useful for interpreting\n" +
		"a prospect's `persona_id` rather than for writing."
)

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
		s.newListCmd(token, userResource, longUserList),
		s.newGetCmd(token, userResource, longUserGet),
	)
	return group
}

// newTemplateCmd builds the template group — inspect email templates referenced
// by sequence steps.
func (s *Service) newTemplateCmd(token string) *cobra.Command {
	group := newGroupCmd("template", "List and look up email templates")
	group.AddCommand(
		s.newListCmd(token, templateResource, longTemplateList),
		s.newGetCmd(token, templateResource, longTemplateGet),
	)
	return group
}

// newStageCmd builds the stage group — ids needed when creating/updating prospects.
func (s *Service) newStageCmd(token string) *cobra.Command {
	group := newGroupCmd("stage", "List prospect stages")
	group.AddCommand(s.newListCmd(token, stageResource, longStageList))
	return group
}

// newPersonaCmd builds the persona group — ids needed when creating/updating prospects.
func (s *Service) newPersonaCmd(token string) *cobra.Command {
	group := newGroupCmd("persona", "List prospect personas")
	group.AddCommand(s.newListCmd(token, personaResource, longPersonaList))
	return group
}
