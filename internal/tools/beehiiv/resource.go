package beehiiv

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newListGroupCmd builds a resource group whose only verb is a
// publication-scoped `list` against GET /publications/{pub}/{resourcePath}.
// segments, custom_fields, tiers, and automations all share this shape.
func (s *Service) newListGroupCmd(token, group, short, resourcePath, listShort string) *cobra.Command {
	cmd := newGroupCmd(group, short)
	list := &cobra.Command{
		Use:         "list",
		Short:       listShort,
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
		"List segments (GET /publications/{pub}/segments)")
}

func (s *Service) newCustomFieldCmd(token string) *cobra.Command {
	return s.newListGroupCmd(token, "custom-field", "Custom fields (list)", "custom_fields",
		"List custom fields (GET /publications/{pub}/custom_fields)")
}

func (s *Service) newTierCmd(token string) *cobra.Command {
	return s.newListGroupCmd(token, "tier", "Subscription tiers (list)", "tiers",
		"List tiers (GET /publications/{pub}/tiers)")
}

func (s *Service) newAutomationCmd(token string) *cobra.Command {
	return s.newListGroupCmd(token, "automation", "Automations (list)", "automations",
		"List automations (GET /publications/{pub}/automations)")
}
