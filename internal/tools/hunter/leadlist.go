package hunter

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newLeadListCmd groups the Leads Lists CRUD
// (GET/POST/PUT/DELETE /leads_lists[/:id]). Free.
func (s *Service) newLeadListCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "lead-list", Short: "Leads lists (list, get, create, update, delete)"}
	cmd.AddCommand(
		s.newLeadListListCmd(key),
		s.newLeadListGetCmd(key),
		s.newLeadListCreateCmd(key),
		s.newLeadListUpdateCmd(key),
		s.newLeadListDeleteCmd(key),
	)
	return cmd
}

func (s *Service) newLeadListListCmd(key string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List leads lists (GET /leads_lists)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", itoa(limit))
			}
			if cmd.Flags().Changed("offset") {
				q.Set("offset", itoa(offset))
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/leads_lists", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "page size")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}

func (s *Service) newLeadListGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one leads list (GET /leads_lists/{id})",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/leads_lists/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "leads list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListCreateCmd(key string) *cobra.Command {
	var name, teamID string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a leads list (POST /leads_lists)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name}
			setBodyIf(body, "team_id", teamID)
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/leads_lists", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "leads list name")
	cmd.Flags().StringVar(&teamID, "team-id", "", "team id to own the list (optional)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newLeadListUpdateCmd(key string) *cobra.Command {
	var id, name string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a leads list (PUT /leads_lists/{id})",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			setBodyIf(body, "name", name)
			resp, err := s.call(cmd.Context(), key, http.MethodPut, "/leads_lists/"+url.PathEscape(id), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "leads list id")
	cmd.Flags().StringVar(&name, "name", "", "new leads list name")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListDeleteCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a leads list (DELETE /leads_lists/{id})",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/leads_lists/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			if len(resp) == 0 {
				return s.emit([]byte(`{"deleted":true}`))
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "leads list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
