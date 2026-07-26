package beehiiv

import (
	"net/http"

	"github.com/spf13/cobra"
)

// The four Longs below belong to the reference lists that one builder
// produces. They live next to it because the builder is what fixes the
// endpoint each describes, and each list answers a different write's "which
// values are legal here" question.
const (
	longSegmentList = "Audience segments defined on the publication: saved audience definitions\n" +
		"rather than lists that can be written to. Nothing here creates a segment\n" +
		"or enumerates its members, and membership is computed by beehiiv rather\n" +
		"than set by hand, so a segment does not update the instant a subscriber\n" +
		"changes. The ids are reference values for reading how a post was\n" +
		"targeted."

	longCustomFieldList = "The custom fields defined on this publication, each with the exact `name`\n" +
		"and data type a write has to use. `subscription create` and\n" +
		"`subscription update` match `custom_fields` entries on that name, and a\n" +
		"name that does not exist here is rejected rather than created on the\n" +
		"fly. Definitions are per publication, so one publication's field list\n" +
		"says nothing about another's."

	longTierList = "The publication's subscription tiers, carrying the exact names\n" +
		"`subscription create` and `subscription update` reference as `tier`.\n" +
		"Tiers are per publication and beehiiv rejects an unknown one, so read\n" +
		"this before moving anybody between them. A publication with no paid\n" +
		"offering has only the free tier."

	longAutomationList = "The automations defined on the publication, each with the `id` that\n" +
		"`subscription create --data '{\"automation_ids\":[…]}'` enrols a new\n" +
		"subscriber into. Enrolment starts a real automation — usually a sequence\n" +
		"of emails to a real person — so read what an automation does before\n" +
		"naming its id. Nothing here creates, edits or stops one."
)

// newListGroupCmd builds a resource group whose only verb is a
// publication-scoped `list` against GET /publications/{pub}/{resourcePath}.
// segments, custom_fields, tiers, and automations all share this shape.
func (s *Service) newListGroupCmd(token, group, short, resourcePath, listShort, listLong string) *cobra.Command {
	cmd := newGroupCmd(group, short)
	list := &cobra.Command{
		Use:         "list",
		Short:       listShort,
		Long:        listLong,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pubID, err := cmd.Flags().GetString("publication-id")
			if err != nil {
				return err
			}
			pub, err := requirePublicationID(pubID)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/publications/"+pub+"/"+resourcePath, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addPublicationFlag(list)
	cmd.AddCommand(list)
	return cmd
}

func (s *Service) newSegmentCmd(token string) *cobra.Command {
	return s.newListGroupCmd(token, "segment", "Audience segments (list)", "segments",
		"List segments (GET /publications/{pub}/segments)", longSegmentList)
}

func (s *Service) newCustomFieldCmd(token string) *cobra.Command {
	return s.newListGroupCmd(token, "custom-field", "Custom fields (list)", "custom_fields",
		"List custom fields (GET /publications/{pub}/custom_fields)", longCustomFieldList)
}

func (s *Service) newTierCmd(token string) *cobra.Command {
	return s.newListGroupCmd(token, "tier", "Subscription tiers (list)", "tiers",
		"List tiers (GET /publications/{pub}/tiers)", longTierList)
}

func (s *Service) newAutomationCmd(token string) *cobra.Command {
	return s.newListGroupCmd(token, "automation", "Automations (list)", "automations",
		"List automations (GET /publications/{pub}/automations)", longAutomationList)
}
