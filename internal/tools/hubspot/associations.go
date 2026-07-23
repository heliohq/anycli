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
		Use:         "create <fromType> <fromId> <toType> <toId>",
		Short:       "Create a default association between two records",
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
		Use:         "list <fromType> <fromId> <toType>",
		Short:       "List associations from one record to a target object type",
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
		Use:         "delete <fromType> <fromId> <toType> <toId>",
		Short:       "Remove all associations between two records",
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
