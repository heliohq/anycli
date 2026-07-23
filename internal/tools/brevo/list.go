package brevo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newListLsCmd builds `brevo list ls` — GET /contacts/lists.
func (s *Service) newListLsCmd(apiKey string) *cobra.Command {
	var limit, offset int
	var sort string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List contact lists (GET /contacts/lists)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("limit", itoa(limit))
			q.Set("offset", itoa(offset))
			if sort != "" {
				q.Set("sort", sort)
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/contacts/lists", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "page size")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order by creation: asc|desc")
	return cmd
}

// newListGetCmd builds `brevo list get` — GET /contacts/lists/{id}.
func (s *Service) newListGetCmd(apiKey string) *cobra.Command {
	var id int
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a contact list (GET /contacts/lists/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/contacts/lists/"+itoa(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "list id (integer)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newListCreateCmd builds `brevo list create` — POST /contacts/lists.
func (s *Service) newListCreateCmd(apiKey string) *cobra.Command {
	var name string
	var folderID int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact list (POST /contacts/lists)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name, "folderId": folderID}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodPost, "/contacts/lists", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "list name")
	cmd.Flags().IntVar(&folderID, "folder-id", 0, "id of the parent folder the list belongs to")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("folder-id")
	return cmd
}

// newListAddContactsCmd builds `brevo list add-contacts` —
// POST /contacts/lists/{id}/contacts/add. Contacts are identified by email or
// by contact id.
func (s *Service) newListAddContactsCmd(apiKey string) *cobra.Command {
	var id int
	var emails []string
	var ids []int
	cmd := &cobra.Command{
		Use:   "add-contacts",
		Short: "Add contacts to a list (POST /contacts/lists/{id}/contacts/add)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if len(emails) > 0 {
				body["emails"] = emails
			}
			if len(ids) > 0 {
				body["ids"] = ids
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodPost, "/contacts/lists/"+itoa(id)+"/contacts/add", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "list id (integer)")
	cmd.Flags().StringArrayVar(&emails, "emails", nil, "contact email to add (repeatable)")
	cmd.Flags().IntSliceVar(&ids, "ids", nil, "contact id to add (repeatable, integer)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
