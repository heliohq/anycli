package hubspot

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAssocGroup builds the associations command group over the CRM v4
// associations API. create makes a default (unlabeled) association; list reads
// associations from one record to a target object type; delete removes all
// associations between two records.
func (s *Service) newAssocGroup(token string) *cobra.Command {
	group := newGroupCmd("assoc", "Manage associations between records (v4)")
	group.AddCommand(
		s.newAssocCreateCmd(token),
		s.newAssocListCmd(token),
		s.newAssocDeleteCmd(token),
	)
	return group
}

// assocV4Base is the CRM v4 objects associations base.
const assocV4Base = "/crm/v4/objects"

func (s *Service) newAssocCreateCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "create <fromType> <fromId> <toType> <toId>",
		Short: "Create a default association between two records",
		Long: "Four positional arguments, where the types are plural API names\n" +
			"(`contacts 1 companies 2`). Creates HubSpot's DEFAULT, unlabeled\n" +
			"association type for that pair — labelled association types cannot be\n" +
			"chosen here. The call is a PUT, so repeating it does not create a second\n" +
			"link.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := assocV4Base + "/" + url.PathEscape(args[0]) + "/" + url.PathEscape(args[1]) +
				"/associations/default/" + url.PathEscape(args[2]) + "/" + url.PathEscape(args[3])
			body, err := s.call(cmd.Context(), token, http.MethodPut, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newAssocListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <fromType> <fromId> <toType>",
		Short: "List associations from one record to a target object type",
		Long: "Three positional arguments: the source type, the source id and the TARGET\n" +
			"object type, all plural API names. This is how a contact's deals or a\n" +
			"company's tickets are found, because records carry no foreign keys. It\n" +
			"returns the associated record IDS and their association types, not the\n" +
			"records — follow up with the target object's own `get`.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := assocV4Base + "/" + url.PathEscape(args[0]) + "/" + url.PathEscape(args[1]) +
				"/associations/" + url.PathEscape(args[2])
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newAssocDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <fromType> <fromId> <toType> <toId>",
		Short: "Remove all associations between two records",
		Long: "Same four positional arguments as `assoc create`, but it removes EVERY\n" +
			"association type between the pair, not only the default one — including\n" +
			"labelled ones this tool cannot re-create. The two records themselves are\n" +
			"untouched.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := assocV4Base + "/" + url.PathEscape(args[0]) + "/" + url.PathEscape(args[1]) +
				"/associations/" + url.PathEscape(args[2]) + "/" + url.PathEscape(args[3])
			body, err := s.call(cmd.Context(), token, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
