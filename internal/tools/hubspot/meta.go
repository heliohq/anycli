package hubspot

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAccountCmd is the whoami / smoke command: it returns the portal (hub)
// details, including the portal id. Used as the L2 harness smoke check.
func (s *Service) newAccountCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "account",
		Short: "Show the connected HubSpot account (portal) details",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/account-info/v3/details", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newOwnerGroup builds the owners command group. list returns owners (optionally
// filtered by email); get returns one owner by id.
func (s *Service) newOwnerGroup(token string) *cobra.Command {
	group := newGroupCmd("owner", "Look up CRM owners for assignment")
	group.AddCommand(
		s.newOwnerListCmd(token),
		s.newOwnerGetCmd(token),
	)
	return group
}

func (s *Service) newOwnerListCmd(token string) *cobra.Command {
	var email string
	var limit int
	var after string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List CRM owners",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if email != "" {
				q.Set("email", email)
			}
			applyPaging(q, limit, after)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/crm/v3/owners", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter owners by email")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a prior response")
	return cmd
}

func (s *Service) newOwnerGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve one owner by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/crm/v3/owners/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newPipelineGroup builds the pipelines command group. list returns the
// pipelines (with stages) for an object type (deals | tickets).
func (s *Service) newPipelineGroup(token string) *cobra.Command {
	group := newGroupCmd("pipeline", "Read deal/ticket pipelines and stages")
	group.AddCommand(s.newPipelineListCmd(token))
	return group
}

func (s *Service) newPipelineListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <objectType>",
		Short: "List pipelines and stages for an object type (deals|tickets)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/crm/v3/pipelines/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newPropertyGroup builds the properties command group for schema discovery.
// list returns all properties of an object type; get returns one by name.
func (s *Service) newPropertyGroup(token string) *cobra.Command {
	group := newGroupCmd("property", "Discover an object type's property schema")
	group.AddCommand(
		s.newPropertyListCmd(token),
		s.newPropertyGetCmd(token),
	)
	return group
}

func (s *Service) newPropertyListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <objectType>",
		Short: "List all properties of an object type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/crm/v3/properties/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newPropertyGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <objectType> <name>",
		Short: "Retrieve one property definition by name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/crm/v3/properties/"+url.PathEscape(args[0])+"/"+url.PathEscape(args[1]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
