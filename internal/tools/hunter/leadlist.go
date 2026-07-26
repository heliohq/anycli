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
		Use:   "list",
		Short: "List leads lists (GET /leads_lists)",
		Long: "The prospect lists on the account, each with its numeric `id`, name and\n" +
			"lead count. That `id` is what `lead list --leads-list-id` filters on and\n" +
			"what `lead create --leads-list-id` files into, so this is the usual first\n" +
			"call before touching leads. `--limit` and `--offset` page it; free, like\n" +
			"every lead command.",
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
		Use:   "get",
		Short: "Get one leads list (GET /leads_lists/{id})",
		Long: "`--id` is a required flag. Returns the list itself — name, owner, team\n" +
			"and how many leads it holds — and not the leads inside it, which come\n" +
			"from `lead list --leads-list-id <id>`.",
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
		Use:   "create",
		Short: "Create a leads list (POST /leads_lists)",
		Long: "`--name` is required and is not checked for uniqueness, so running this\n" +
			"twice leaves two lists with the same name and different ids — read\n" +
			"`lead-list list` first if the list may already exist. `--team-id` hands\n" +
			"ownership to a team instead of the individual account, which is what\n" +
			"makes it visible to colleagues. Keep the returned `id`; the lead\n" +
			"commands take it, not the name.",
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
		Use:   "update",
		Short: "Update a leads list (PUT /leads_lists/{id})",
		Long: "A rename, and nothing more: `--id` is required and `--name` is the only\n" +
			"mutable field, so a call without `--name` sends an empty body and\n" +
			"changes nothing. Membership and ownership are untouched — leads keep\n" +
			"their place across a rename, and a list cannot be handed to a different\n" +
			"team here.",
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
		Use:   "delete",
		Short: "Delete a leads list (DELETE /leads_lists/{id})",
		Long: "`--id` is required. Hunter answers 204 with no body, so this prints\n" +
			"`{\"deleted\":true}` as the receipt. There is no undo and the id stops\n" +
			"resolving, so anything filtered by `--leads-list-id` afterwards returns\n" +
			"nothing — capture the membership with `lead list --leads-list-id <id>`\n" +
			"first if it matters.",
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
